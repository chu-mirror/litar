package litar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type agent struct {
	server    *mcp.Server
	mu        sync.Mutex
	world     *World
	uris      []string
	tools     bool
	published bool
}

var fixedResources = []struct {
	name, desc, uri, mime string
}{
	{"composer path", "the path to the composer directory", "path://composer", "text/plain"},
	{"nodes", "the node tree", "tree://node", "application/json"},
}

// chunk_reference stands for the design name "chunk reference".
// An RFC 6570 variable name cannot contain a space.
var resourceTemplates = []struct {
	name, desc, uri, mime string
}{
	{"archive file path", "the path to the node's archive", "path://node/{node}", "text/plain"},
	{"chunks", "the chunk tree organized by modules under the node", "tree://chunk/{node}", "application/json"},
	{"items", "the item tree under the node", "tree://item/{node}", "application/json"},
	{"files", "the file tree under the node", "tree://file/{node}", "application/json"},
	{"node blocks", "the blocks in the node", "blocks://node/{node}", "text/plain"},
	{"applied blocks", "the blocks applied to the chunk reference", "blocks://apply/{node}/{chunk_reference}", "text/plain"},
	{"effective blocks", "the effective blocks of the chunk reference", "blocks://effective/{node}/{chunk_reference}", "text/plain"},
	{"dependent blocks", "the blocks depending on a chunk reference", "blocks://depend/{node}/{chunk_reference}", "text/plain"},
	{"related blocks", "the blocks referring to the item", "blocks://refer/{node}/{item}", "text/plain"},
	{"file", "the file of the path", "file://{node}/{path}", "text/plain"},
}

func newAgent() *agent {
	a := &agent{}
	a.server = mcp.NewServer(
		&mcp.Implementation{Name: "litar", Version: "0.1.0"},
		&mcp.ServerOptions{
			SubscribeHandler:   func(context.Context, *mcp.SubscribeRequest) error { return nil },
			UnsubscribeHandler: func(context.Context, *mcp.UnsubscribeRequest) error { return nil },
			Capabilities: &mcp.ServerCapabilities{
				Tools:     &mcp.ToolCapabilities{ListChanged: true},
				Resources: &mcp.ResourceCapabilities{ListChanged: true, Subscribe: true},
			},
		},
	)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "create_composed_archive",
		Description: "Create a new composed archive, and use that composed archive.",
	}, a.createComposedArchive)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "specify_composed_archive",
		Description: "Specify the composed archive.",
	}, a.specifyComposer)
	return a
}

func (a *agent) serve(ctx context.Context) error {
	return a.server.Run(ctx, &mcp.StdioTransport{})
}

type specifyIn struct {
	Path string `json:"path" jsonschema:"path to the composer file or the composed archive directory"`
}

type createArchiveIn struct {
	Path string `json:"path" jsonschema:"directory of the new composed archive"`
}

type createIn struct {
	Name        string `json:"name" jsonschema:"node name"`
	Archive     string `json:"archive" jsonschema:"archive path"`
	Description string `json:"description" jsonschema:"node description"`
	// A pointer so JSON null unmarshals. The property stays required:
	// an absent base is not null. createNodeSchema publishes anyOf
	// instead of the inferred type array.
	Base *string `json:"base" jsonschema:"base node name, empty or null to create the root"`
}

type deleteNodeIn struct {
	Name string `json:"name" jsonschema:"node name"`
}

type deleteChunkIn struct {
	Node      string `json:"node" jsonschema:"node name"`
	Reference string `json:"reference" jsonschema:"chunk reference"`
}

type deleteFileIn struct {
	Node string `json:"node" jsonschema:"node name"`
	Path string `json:"path" jsonschema:"file path"`
}

type evalIn struct {
	Node       string `json:"node" jsonschema:"node name"`
	Expression string `json:"expression" jsonschema:"expression to evaluate"`
}

type applyIn struct {
	Node string `json:"node" jsonschema:"node name"`
	Text string `json:"text" jsonschema:"block text to append"`
}

type emptyIn struct{}

func (a *agent) ensureTools() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tools {
		return
	}
	a.tools = true
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "create_node",
		Description: "Create a node. An empty or null base creates the root, and only when the composer has none. Create an empty archive if the file does not exist.",
		InputSchema: createNodeSchema(),
	}, a.createNode)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "delete_node",
		Description: "Delete a node and every node based on it, and delete their archive files.",
	}, a.deleteNode)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "delete_chunk",
		Description: "Delete the blocks applied to a chunk reference from the node's own archive.",
	}, a.deleteChunk)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "delete_file",
		Description: "Delete a file and the blocks applied to its path from the node's own archive.",
	}, a.deleteFile)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "evaluate_expression",
		Description: "Evaluate an expression at a node.",
	}, a.evaluate)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "apply_block",
		Description: "Append a block to a node whose archive path starts with '.' or '..'.",
	}, a.applyBlock)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "dump_archives",
		Description: "Rewrite each archive, keeping only the effective blocks of each chunk reference.",
	}, a.dumpArchives)
}

// createNodeSchema is the inferred object schema with base rewritten.
// A *string infers "type": ["null", "string"]. Some clients accept only
// one type per schema, so null and string are separate anyOf branches.
func createNodeSchema() *jsonschema.Schema {
	s, err := jsonschema.For[createIn](nil)
	if err != nil {
		panic(err)
	}
	prop := s.Properties["base"]
	s.Properties["base"] = &jsonschema.Schema{
		Description: prop.Description,
		AnyOf: []*jsonschema.Schema{
			{Type: "null"},
			{Type: "string"},
		},
	}
	return s
}

func toolErr(err error) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}, nil, nil
}

func toolText(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, map[string]any{"text": text}, nil
}

func (a *agent) useWorld(w *World) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	a.world = w
	a.mu.Unlock()
	a.ensureTools()
	a.refresh()
	return toolText(w.Composer)
}

func (a *agent) createComposedArchive(ctx context.Context, _ *mcp.CallToolRequest, in createArchiveIn) (*mcp.CallToolResult, any, error) {
	if in.Path == "" {
		return toolErr(errString("missing path"))
	}
	w, err := CreateComposedArchive(in.Path)
	if err != nil {
		return toolErr(err)
	}
	return a.useWorld(w)
}

func (a *agent) specifyComposer(ctx context.Context, _ *mcp.CallToolRequest, in specifyIn) (*mcp.CallToolResult, any, error) {
	if in.Path == "" {
		return toolErr(errString("missing path"))
	}
	w, err := SpecifyComposer(in.Path)
	if err != nil {
		return toolErr(err)
	}
	return a.useWorld(w)
}

type errString string

func (e errString) Error() string { return string(e) }

func (a *agent) createNode(ctx context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.world == nil {
		return toolErr(errString("composer is not specified"))
	}
	base := ""
	if in.Base != nil {
		base = *in.Base
	}
	if err := a.world.CreateNode(in.Name, in.Archive, in.Description, base); err != nil {
		return toolErr(err)
	}
	a.mu.Unlock()
	a.refresh()
	a.mu.Lock()
	return toolText(in.Name)
}

func (a *agent) deleteNode(ctx context.Context, _ *mcp.CallToolRequest, in deleteNodeIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	if a.world == nil {
		a.mu.Unlock()
		return toolErr(errString("composer is not specified"))
	}
	if err := a.world.DeleteNode(in.Name); err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	a.mu.Unlock()
	a.refresh()
	return toolText(in.Name)
}

func (a *agent) deleteChunk(ctx context.Context, _ *mcp.CallToolRequest, in deleteChunkIn) (*mcp.CallToolResult, any, error) {
	ref, err := ParseChunkRef(in.Reference)
	if err != nil {
		return toolErr(err)
	}
	a.mu.Lock()
	if a.world == nil {
		a.mu.Unlock()
		return toolErr(errString("composer is not specified"))
	}
	if err := a.world.DeleteChunk(in.Node, ref); err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	a.mu.Unlock()
	a.refresh()
	return toolText(in.Reference)
}

func (a *agent) deleteFile(ctx context.Context, _ *mcp.CallToolRequest, in deleteFileIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	if a.world == nil {
		a.mu.Unlock()
		return toolErr(errString("composer is not specified"))
	}
	if err := a.world.DeleteFile(in.Node, in.Path); err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	a.mu.Unlock()
	a.refresh()
	return toolText(in.Path)
}

func (a *agent) evaluate(ctx context.Context, _ *mcp.CallToolRequest, in evalIn) (*mcp.CallToolResult, any, error) {
	ref, filters, err := ParsePrint(in.Expression)
	if err != nil {
		return toolErr(err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.world == nil {
		return toolErr(errString("composer is not specified"))
	}
	text, err := dispatch(a.world, in.Node, "print", in.Expression, nil)
	if err != nil {
		return toolErr(err)
	}
	_ = ref
	_ = filters
	return toolText(text)
}

func (a *agent) applyBlock(ctx context.Context, _ *mcp.CallToolRequest, in applyIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	if a.world == nil {
		a.mu.Unlock()
		return toolErr(errString("composer is not specified"))
	}
	if err := a.world.ApplyBlock(in.Node, in.Text); err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	a.mu.Unlock()
	a.refresh()
	return toolText(in.Text)
}

func (a *agent) dumpArchives(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	if a.world == nil {
		a.mu.Unlock()
		return toolErr(errString("composer is not specified"))
	}
	if err := a.world.DumpArchives(); err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	a.mu.Unlock()
	a.refresh()
	return toolText("ok")
}

func (a *agent) handle(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.world == nil {
		return nil, errString("composer is not specified")
	}
	text, mime, blob, err := a.render(req.Params.URI)
	if err != nil {
		return nil, err
	}
	contents := &mcp.ResourceContents{URI: req.Params.URI, MIMEType: mime}
	if blob != nil {
		contents.Blob = blob
	} else {
		contents.Text = text
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{contents}}, nil
}

func (a *agent) refresh() {
	a.mu.Lock()
	next := a.listURIsLocked()
	prev := a.uris
	a.uris = next
	publish := a.world != nil && !a.published
	if publish {
		a.published = true
	}
	a.mu.Unlock()

	// The two resources and the nine templates are registered once.
	// A later mutation changes their contents, not the lists.
	if publish {
		for _, r := range fixedResources {
			a.server.AddResource(&mcp.Resource{
				URI:         r.uri,
				Name:        r.name,
				Description: r.desc,
				MIMEType:    r.mime,
			}, a.handle)
		}
		for _, t := range resourceTemplates {
			a.server.AddResourceTemplate(&mcp.ResourceTemplate{
				URITemplate: t.uri,
				Name:        t.name,
				Description: t.desc,
				MIMEType:    t.mime,
			}, a.handle)
		}
	}

	prevSet := map[string]bool{}
	for _, u := range prev {
		prevSet[u] = true
	}
	union := append([]string{}, prev...)
	for _, u := range next {
		if !prevSet[u] {
			union = append(union, u)
		}
	}
	if len(union) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for _, u := range union {
			_ = a.server.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: u})
		}
	}()
}

func mimeOf(uri string) string {
	if strings.HasPrefix(uri, "tree:") {
		return "application/json"
	}
	return "text/plain"
}

func escJoin(parts ...string) string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = url.PathEscape(p)
	}
	return strings.Join(out, "/")
}

// listURIsLocked lists concrete URIs whose contents may have changed.
// path://composer and tree://node are registered resources.
// Every other URI is a read of a resource template.
func (a *agent) listURIsLocked() []string {
	if a.world == nil {
		return nil
	}
	var uris []string
	add := func(u string) { uris = append(uris, u) }
	add("path://composer")
	add("tree://node")
	for _, n := range a.world.Nodes {
		add("path://node/" + escJoin(n.Name))
		add("tree://chunk/" + escJoin(n.Name))
		add("tree://item/" + escJoin(n.Name))
		add("tree://file/" + escJoin(n.Name))
		add("blocks://node/" + escJoin(n.Name))
		chain, err := a.world.Chain(n.Name)
		if err != nil {
			continue
		}
		seenRef := map[string]ChunkRef{}
		for _, b := range chain {
			if b.Ref.Chunk == "" {
				continue
			}
			k := b.Ref.key()
			if _, ok := seenRef[k]; ok {
				continue
			}
			seenRef[k] = b.Ref
			ref := b.Ref.String()
			add("blocks://apply/" + escJoin(n.Name, ref))
			add("blocks://effective/" + escJoin(n.Name, ref))
			add("blocks://depend/" + escJoin(n.Name, ref))
		}
		seenItem := map[string]Item{}
		for _, b := range chain {
			for _, it := range b.Items {
				k := formatItem(it)
				if _, ok := seenItem[k]; ok {
					continue
				}
				seenItem[k] = it
				add("blocks://refer/" + escJoin(n.Name, k))
			}
		}
		maps, err := a.world.Mappings(n.Name)
		if err != nil {
			continue
		}
		cat := buildCatalog(chain, maps)
		for _, path := range cat.order {
			add("file://" + escJoin(n.Name, path))
		}
	}
	return uris
}

func (a *agent) render(uri string) (text, mime string, blob []byte, err error) {
	mime = mimeOf(uri)
	switch {
	case uri == "path://composer":
		return filepath.Dir(a.world.Composer), mime, nil, nil
	case uri == "tree://node":
		raw, err := json.Marshal(nodeTree(a.world))
		if err != nil {
			return "", "", nil, err
		}
		return string(raw), mime, nil, nil
	case strings.HasPrefix(uri, "path://node/"):
		name, err := unescapeComp(strings.TrimPrefix(uri, "path://node/"))
		if err != nil {
			return "", "", nil, err
		}
		n, err := a.world.lookup(name)
		if err != nil {
			return "", "", nil, err
		}
		return n.Archive, mime, nil, nil
	case strings.HasPrefix(uri, "tree://chunk/"):
		name, err := unescapeComp(strings.TrimPrefix(uri, "tree://chunk/"))
		if err != nil {
			return "", "", nil, err
		}
		chain, err := a.world.Chain(name)
		if err != nil {
			return "", "", nil, err
		}
		maps, err := a.world.Mappings(name)
		if err != nil {
			return "", "", nil, err
		}
		tree, err := chunkTree(chain, maps)
		if err != nil {
			return "", "", nil, err
		}
		raw, err := json.Marshal(tree)
		if err != nil {
			return "", "", nil, err
		}
		return string(raw), mime, nil, nil
	case strings.HasPrefix(uri, "tree://item/"):
		name, err := unescapeComp(strings.TrimPrefix(uri, "tree://item/"))
		if err != nil {
			return "", "", nil, err
		}
		chain, err := a.world.Chain(name)
		if err != nil {
			return "", "", nil, err
		}
		maps, err := a.world.Mappings(name)
		if err != nil {
			return "", "", nil, err
		}
		tree, err := itemTree(chain, maps)
		if err != nil {
			return "", "", nil, err
		}
		raw, err := json.Marshal(tree)
		if err != nil {
			return "", "", nil, err
		}
		return string(raw), mime, nil, nil
	case strings.HasPrefix(uri, "tree://file/"):
		name, err := unescapeComp(strings.TrimPrefix(uri, "tree://file/"))
		if err != nil {
			return "", "", nil, err
		}
		chain, err := a.world.Chain(name)
		if err != nil {
			return "", "", nil, err
		}
		maps, err := a.world.Mappings(name)
		if err != nil {
			return "", "", nil, err
		}
		raw, err := json.Marshal(fileTree(chain, maps))
		if err != nil {
			return "", "", nil, err
		}
		return string(raw), mime, nil, nil
	case strings.HasPrefix(uri, "blocks://node/"):
		name, err := unescapeComp(strings.TrimPrefix(uri, "blocks://node/"))
		if err != nil {
			return "", "", nil, err
		}
		n, err := a.world.lookup(name)
		if err != nil {
			return "", "", nil, err
		}
		return FormatBlocks(n.Own), mime, nil, nil
	case strings.HasPrefix(uri, "blocks://apply/"),
		strings.HasPrefix(uri, "blocks://effective/"),
		strings.HasPrefix(uri, "blocks://depend/"),
		strings.HasPrefix(uri, "blocks://refer/"):
		return a.renderBlocks(uri, mime)
	case strings.HasPrefix(uri, "file://"):
		return a.renderFile(uri, mime)
	default:
		return "", "", nil, errString("unknown resource")
	}
}

func unescapeComp(s string) (string, error) {
	return url.PathUnescape(s)
}

func splitResource(rest string) (node, arg string, err error) {
	nodeEsc, argEsc, ok := strings.Cut(rest, "/")
	if !ok {
		return "", "", errString("unknown resource")
	}
	node, err = url.PathUnescape(nodeEsc)
	if err != nil {
		return "", "", err
	}
	arg, err = url.PathUnescape(argEsc)
	if err != nil {
		return "", "", err
	}
	return node, arg, nil
}

func (a *agent) renderBlocks(uri, mime string) (string, string, []byte, error) {
	var kind, rest string
	switch {
	case strings.HasPrefix(uri, "blocks://apply/"):
		kind, rest = "literal", strings.TrimPrefix(uri, "blocks://apply/")
	case strings.HasPrefix(uri, "blocks://effective/"):
		kind, rest = "effective", strings.TrimPrefix(uri, "blocks://effective/")
	case strings.HasPrefix(uri, "blocks://depend/"):
		kind, rest = "depend", strings.TrimPrefix(uri, "blocks://depend/")
	default:
		kind, rest = "reference", strings.TrimPrefix(uri, "blocks://refer/")
	}
	node, arg, err := splitResource(rest)
	if err != nil {
		return "", "", nil, err
	}
	text, err := dispatch(a.world, node, kind, arg, nil)
	if err != nil {
		return "", "", nil, err
	}
	return text, mime, nil, nil
}

func (a *agent) renderFile(uri, mime string) (string, string, []byte, error) {
	node, path, err := splitResource(strings.TrimPrefix(uri, "file://"))
	if err != nil {
		return "", "", nil, err
	}
	chain, err := a.world.Chain(node)
	if err != nil {
		return "", "", nil, err
	}
	maps, err := a.world.Mappings(node)
	if err != nil {
		return "", "", nil, err
	}
	cat := buildCatalog(chain, maps)
	f := cat.files[path]
	if f == nil {
		return "", "", nil, errString("file '" + path + "' is not defined")
	}
	if f.archive && f.mapping {
		return "", "", nil, errString("file '" + path + "' is defined as both an archive file and a mapping")
	}
	var data []byte
	if f.mapping && !f.archive {
		data, err = readOrigin(f.origin)
		if err != nil {
			return "", "", nil, err
		}
	} else {
		ev := NewEvaluator(chain, maps, nil)
		text, err := ev.Eval(ChunkRef{Chunk: Name(path)}, nil)
		if err != nil {
			return "", "", nil, err
		}
		data = []byte(text)
	}
	if utf8.Valid(data) {
		return string(data), mime, nil, nil
	}
	return "", "application/octet-stream", data, nil
}

func readOrigin(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errString("could not read '" + path + "'")
	}
	return data, nil
}

type treeJSON struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	SubTrees    []treeJSON `json:"sub-trees"`
}

func emptySub() []treeJSON {
	return []treeJSON{}
}

type nest struct {
	order  []nestEntry
	leaves map[string]bool
	subs   map[string]*nest
}

type nestEntry struct {
	name   string
	module bool
}

func newNest() *nest {
	return &nest{leaves: map[string]bool{}, subs: map[string]*nest{}}
}

func (n *nest) addLeaf(name string) {
	if n.leaves[name] {
		return
	}
	n.leaves[name] = true
	n.order = append(n.order, nestEntry{name: name})
}

func (n *nest) addSub(name string) *nest {
	if s := n.subs[name]; s != nil {
		return s
	}
	s := newNest()
	n.subs[name] = s
	n.order = append(n.order, nestEntry{name: name, module: true})
	return s
}

func (n *nest) tree(name, desc string) treeJSON {
	return n.treeWith(name, desc, nil, nil, nil)
}

func (n *nest) treeWith(name, desc string, path []Name, nodeDesc func([]Name) string, leafDesc func([]Name, string) string) treeJSON {
	t := treeJSON{Name: name, Description: desc, SubTrees: emptySub()}
	for _, e := range n.order {
		if e.module {
			sub := append(append([]Name(nil), path...), Name(e.name))
			d := ""
			if nodeDesc != nil {
				d = nodeDesc(sub)
			}
			t.SubTrees = append(t.SubTrees, n.subs[e.name].treeWith(e.name, d, sub, nodeDesc, leafDesc))
		} else {
			d := ""
			if leafDesc != nil {
				d = leafDesc(path, e.name)
			}
			t.SubTrees = append(t.SubTrees, treeJSON{Name: e.name, Description: d, SubTrees: emptySub()})
		}
	}
	return t
}

// A reserved chunk lives in the Module, Chunk, Domain, or Index namespace.
// Its module path starts with that name. The remaining components, then the
// chunk name, are the module, chunk, domain, or item the chunk describes.
func reservedNamespace(mod ModulePath) (Name, bool) {
	if len(mod) == 0 {
		return "", false
	}
	switch mod[0] {
	case "Module", "Chunk", "Domain", "Index":
		return mod[0], true
	default:
		return "", false
	}
}

type metaInfo struct {
	modules map[string]string
	chunks  map[string]string
	domains map[string]string
	items   map[string]string
}

func newMeta() metaInfo {
	return metaInfo{
		modules: map[string]string{},
		chunks:  map[string]string{},
		domains: map[string]string{},
		items:   map[string]string{},
	}
}

func pathKey(path []Name) string {
	return formatModule(path)
}

func leafKey(path []Name, name string) string {
	return pathKey(path) + "\x00" + name
}

func metaDescription(text string) (string, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	var doc struct {
		Description string `json:"description"`
	}
	if err := dec.Decode(&doc); err != nil {
		return "", err
	}
	if dec.More() {
		return "", fmt.Errorf("trailing data")
	}
	return doc.Description, nil
}

func describe(blocks []Block, maps []Mapping) (metaInfo, error) {
	meta := newMeta()
	ev := NewEvaluator(blocks, maps, nil)
	seen := map[string]bool{}
	for _, b := range blocks {
		ns, ok := reservedNamespace(b.Ref.Module)
		if !ok || b.Ref.Chunk == "" || len(b.Ref.Labels) != 0 {
			continue
		}
		ref := ChunkRef{Module: copyModule(b.Ref.Module), Chunk: b.Ref.Chunk}
		if seen[ref.key()] {
			continue
		}
		seen[ref.key()] = true
		text, err := ev.Eval(ref, nil)
		if err != nil {
			return metaInfo{}, err
		}
		desc, err := metaDescription(text)
		if err != nil {
			return metaInfo{}, fmt.Errorf("meta chunk '%s': %w", ref.String(), err)
		}
		target := append(append([]Name{}, b.Ref.Module[1:]...), b.Ref.Chunk)
		switch ns {
		case "Module":
			meta.modules[pathKey(target)] = desc
		case "Domain":
			meta.domains[pathKey(target)] = desc
		case "Chunk":
			mod, name := splitTail(target)
			meta.chunks[leafKey(mod, string(name))] = desc
		case "Index":
			mod, name := splitTail(target)
			meta.items[leafKey(mod, string(name))] = desc
		}
	}
	return meta, nil
}

func splitTail(path []Name) ([]Name, Name) {
	if len(path) == 0 {
		return nil, ""
	}
	return path[:len(path)-1], path[len(path)-1]
}

func chunkTree(blocks []Block, maps []Mapping) (treeJSON, error) {
	meta, err := describe(blocks, maps)
	if err != nil {
		return treeJSON{}, err
	}
	root := newNest()
	for _, b := range blocks {
		if _, reserved := reservedNamespace(b.Ref.Module); reserved {
			continue
		}
		cur := root
		for _, part := range b.Ref.Module {
			cur = cur.addSub(string(part))
		}
		if b.Ref.Chunk != "" {
			cur.addLeaf(string(b.Ref.Chunk))
		}
	}
	tree := root.treeWith("/", meta.modules[pathKey(nil)], nil,
		func(path []Name) string { return meta.modules[pathKey(path)] },
		func(path []Name, name string) string { return meta.chunks[leafKey(path, name)] },
	)
	return tree, nil
}

func fileTree(blocks []Block, mappings []Mapping) treeJSON {
	cat := buildCatalog(blocks, mappings)
	root := newNest()
	for _, path := range cat.order {
		parts := strings.Split(path, "/")
		cur := root
		for i, part := range parts {
			if i == len(parts)-1 {
				cur.addLeaf(part)
				break
			}
			cur = cur.addSub(part)
		}
	}
	return root.tree("/", "")
}

func itemTree(blocks []Block, maps []Mapping) (treeJSON, error) {
	meta, err := describe(blocks, maps)
	if err != nil {
		return treeJSON{}, err
	}
	root := newNest()
	for _, b := range blocks {
		for _, it := range b.Items {
			cur := root
			for i, name := range it {
				if i == len(it)-1 {
					cur.addLeaf(string(name))
					break
				}
				cur = cur.addSub(string(name))
			}
		}
	}
	tree := root.treeWith("/", meta.domains[pathKey(nil)], nil,
		func(path []Name) string { return meta.domains[pathKey(path)] },
		func(path []Name, name string) string { return meta.items[leafKey(path, name)] },
	)
	return tree, nil
}

func nodeTree(w *World) treeJSON {
	children := map[string][]*Node{}
	var root *Node
	for i := range w.Nodes {
		n := &w.Nodes[i]
		if n.Base == "" {
			root = n
			continue
		}
		children[n.Base] = append(children[n.Base], n)
	}
	if root == nil {
		return treeJSON{Name: "", Description: "", SubTrees: emptySub()}
	}
	var build func(*Node) treeJSON
	build = func(n *Node) treeJSON {
		t := treeJSON{Name: n.Name, Description: n.Description, SubTrees: emptySub()}
		for _, c := range children[n.Name] {
			t.SubTrees = append(t.SubTrees, build(c))
		}
		return t
	}
	return build(root)
}
