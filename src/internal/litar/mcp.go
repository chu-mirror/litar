package litar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var fixedResources = []struct {
	name, desc, uri, mime string
}{
	{"compositions", "a list of editable compositions", "list://composition", "application/json"},
}

var resourceTemplates = []struct {
	name, desc, uri, mime string
}{
	{"nodes", "the node tree under the composition", "tree://node/{composition}", "application/json"},
	{"chunks", "the chunk tree organized by modules under the node", "tree://chunk/{composition}/{node}", "application/json"},
	{"items", "the item tree under the node", "tree://item/{composition}/{node}", "application/json"},
	{"files", "the file tree under the node", "tree://file/{composition}/{node}", "application/json"},
	{"node blocks", "the blocks in the node", "blocks://node/{composition}/{node}", "text/plain"},
	{"specializations", "the specializations on the chunk", "tree://specialization/{composition}/{node}/{chunk}", "application/json"},
	{"applied blocks", "the blocks applied to the chunk reference", "blocks://apply/{composition}/{node}/{chunk}/{specialization}", "text/plain"},
	{"effective blocks", "the effective blocks of the chunk reference", "blocks://effective/{composition}/{node}/{chunk}/{specialization}", "text/plain"},
	{"dependent blocks", "the blocks depending on a chunk reference", "blocks://depend/{composition}/{node}/{chunk}/{specialization}", "text/plain"},
	{"related blocks", "the blocks referring to the item", "blocks://refer/{composition}/{node}/{item}", "text/plain"},
	{"file", "the file of the path", "file://{composition}/{node}/{path}", "text/plain"},
}

type agent struct {
	server *mcp.Server
	mu     sync.Mutex
	worlds map[string]*World
	order  []string
	uris   []string
}

type importIn struct {
	Path string `json:"path" jsonschema:"path to the composer file or the composition directory"`
}

type createArchiveIn struct {
	Path string `json:"path" jsonschema:"directory of the new composition"`
	// A pointer so JSON null unmarshals. The property stays required:
	// an absent ground is not null. nullableStringSchema publishes anyOf
	// instead of the inferred type array.
	Ground      *string `json:"ground" jsonschema:"name of the ground composition, or null"`
	Description string  `json:"description,omitempty" jsonschema:"description of the composition"`
}

type createIn struct {
	Composition int    `json:"composition" jsonschema:"id of the composition"`
	Name        string `json:"name" jsonschema:"node name"`
	Archive     string `json:"archive" jsonschema:"archive path; the file is created empty"`
	Description string `json:"description" jsonschema:"node description"`
	// Null creates the root. A number is the id of the parent node.
	Base *int `json:"base" jsonschema:"id of the parent node, or null to create the root"`
}

type deleteNodeIn struct {
	Composition int `json:"composition" jsonschema:"id of the composition"`
	Node        int `json:"node" jsonschema:"id of the node"`
}

type deleteChunkIn struct {
	Composition int `json:"composition" jsonschema:"id of the composition"`
	Node        int `json:"node" jsonschema:"id of the node"`
	Chunk       int `json:"chunk" jsonschema:"id of the chunk"`
}

type deleteFileIn struct {
	Composition int `json:"composition" jsonschema:"id of the composition"`
	Node        int `json:"node" jsonschema:"id of the node"`
	Path        int `json:"path" jsonschema:"id of the file"`
}

type evalIn struct {
	Composition int    `json:"composition" jsonschema:"id of the composition"`
	Node        int    `json:"node" jsonschema:"id of the node"`
	Expression  string `json:"expression" jsonschema:"expression to evaluate"`
}

type applyIn struct {
	Composition int    `json:"composition" jsonschema:"id of the composition"`
	Node        int    `json:"node" jsonschema:"id of the node"`
	Text        string `json:"text" jsonschema:"block text to append"`
}

type errString string

type listJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ID          int    `json:"id"`
}

type treeJSON struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	ID          int        `json:"id"`
	SubTrees    []treeJSON `json:"sub-trees"`
	// Leaf is true when this node is a chunk, file, or item.
	// Module and directory branches are not leaves.
	// Every node and every specialization is addressable, so those trees set it too.
	Leaf bool `json:"-"`
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

type metaInfo struct {
	modules map[string]string
	chunks  map[string]string
	domains map[string]string
	items   map[string]string
	specs   map[string]string
}

type treePlace struct {
	names []string
	leaf  bool
}

func (a *agent) serve(ctx context.Context) error {
	return a.server.Run(ctx, &mcp.StdioTransport{})
}

func (a *agent) addWorldLocked(w *World) {
	if _, ok := a.worlds[w.Dir]; !ok {
		a.order = append(a.order, w.Dir)
	}
	a.worlds[w.Dir] = w
}

func (a *agent) addWorld(w *World) {
	a.mu.Lock()
	a.addWorldLocked(w)
	a.mu.Unlock()
}

func (a *agent) useWorld(w *World) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	a.addWorldLocked(w)
	id := -1
	for i, c := range a.compositions() {
		if c.Dir == w.Dir {
			id = i
			break
		}
	}
	a.mu.Unlock()
	a.refresh()
	if id < 0 {
		return toolErr(errString("unknown composition"))
	}
	return toolText(strconv.Itoa(id))
}

// compositions is the imported compositions in list order.
// The caller holds a.mu.
func (a *agent) compositions() []*World {
	out := make([]*World, 0, len(a.order))
	for _, dir := range a.order {
		if w := a.worlds[dir]; w != nil {
			out = append(out, w)
		}
	}
	return out
}

// compositionByID resolves an id from list://composition.
// The caller holds a.mu.
func (a *agent) compositionByID(id int) (*World, error) {
	comps := a.compositions()
	if id < 0 || id >= len(comps) {
		return nil, fmt.Errorf("unknown composition %d", id)
	}
	return comps[id], nil
}

func (a *agent) importComposition(ctx context.Context, _ *mcp.CallToolRequest, in importIn) (*mcp.CallToolResult, any, error) {
	if in.Path == "" {
		return toolErr(errString("missing path"))
	}
	w, err := SpecifyComposer(in.Path)
	if err != nil {
		return toolErr(err)
	}
	return a.useWorld(w)
}

func (a *agent) createComposition(ctx context.Context, _ *mcp.CallToolRequest, in createArchiveIn) (*mcp.CallToolResult, any, error) {
	if in.Path == "" {
		return toolErr(errString("missing path"))
	}
	ground := ""
	if in.Ground != nil {
		ground = *in.Ground
	}
	w, err := CreateComposedArchive(in.Path, ground)
	if err != nil {
		return toolErr(err)
	}
	if in.Description != "" {
		w.Description = in.Description
		if err := w.save(); err != nil {
			return toolErr(err)
		}
	}
	return a.useWorld(w)
}

func (e errString) Error() string { return string(e) }

func (a *agent) createNode(ctx context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	w, err := a.compositionByID(in.Composition)
	if err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	var base []string
	if in.Base != nil {
		parent, err := nodeByID(w, *in.Base)
		if err != nil {
			a.mu.Unlock()
			return toolErr(err)
		}
		base = nodePath(*parent)
	}
	if err := w.CreateNode(in.Name, in.Archive, in.Description, base); err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	id, err := nodeID(w, formatNodePath(append(base, in.Name)))
	a.mu.Unlock()
	if err != nil {
		return toolErr(err)
	}
	a.refresh()
	return toolText(strconv.Itoa(id))
}

func (a *agent) deleteNode(ctx context.Context, _ *mcp.CallToolRequest, in deleteNodeIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	w, n, err := a.worldNode(in.Composition, in.Node)
	if err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	if err := w.DeleteNode(formatNodePath(nodePath(*n))); err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	a.mu.Unlock()
	a.refresh()
	return toolText(strconv.Itoa(in.Node))
}

func (a *agent) deleteChunk(ctx context.Context, _ *mcp.CallToolRequest, in deleteChunkIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	w, n, err := a.worldNode(in.Composition, in.Node)
	if err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	chain, maps, err := a.nodeChain(w, in.Node)
	if err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	ref, err := chunkByID(chain, maps, in.Chunk)
	if err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	if err := w.DeleteChunkAll(formatNodePath(nodePath(*n)), ref); err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	a.mu.Unlock()
	a.refresh()
	return toolText(strconv.Itoa(in.Chunk))
}

func (a *agent) deleteFile(ctx context.Context, _ *mcp.CallToolRequest, in deleteFileIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	w, n, err := a.worldNode(in.Composition, in.Node)
	if err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	chain, maps, err := a.nodeChain(w, in.Node)
	if err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	path, err := fileByID(chain, maps, in.Path)
	if err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	if err := w.DeleteFile(formatNodePath(nodePath(*n)), path); err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	a.mu.Unlock()
	a.refresh()
	return toolText(strconv.Itoa(in.Path))
}

func (a *agent) evaluate(ctx context.Context, _ *mcp.CallToolRequest, in evalIn) (*mcp.CallToolResult, any, error) {
	ref, filters, err := ParsePrint(in.Expression)
	if err != nil {
		return toolErr(err)
	}
	a.mu.Lock()
	w, err := a.compositionByID(in.Composition)
	if err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	chain, maps, err := a.nodeChain(w, in.Node)
	a.mu.Unlock()
	if err != nil {
		return toolErr(err)
	}
	text, err := evalReporting(chain, maps, ref, filters)
	if err != nil {
		return toolErr(err)
	}
	return toolText(text)
}

func (a *agent) applyBlock(ctx context.Context, _ *mcp.CallToolRequest, in applyIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	w, n, err := a.worldNode(in.Composition, in.Node)
	if err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	if err := w.ApplyBlock(formatNodePath(nodePath(*n)), in.Text); err != nil {
		a.mu.Unlock()
		return toolErr(err)
	}
	a.mu.Unlock()
	a.refresh()
	return toolText(in.Text)
}

// worldNode resolves a composition id and a node id.
// The caller holds a.mu.
func (a *agent) worldNode(composition, node int) (*World, *Node, error) {
	w, err := a.compositionByID(composition)
	if err != nil {
		return nil, nil, err
	}
	n, err := nodeByID(w, node)
	if err != nil {
		return nil, nil, err
	}
	return w, n, nil
}

func (a *agent) handle(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
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
	a.mu.Unlock()

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

// listURIsLocked lists concrete URIs whose contents may have changed.
// list://composition is the registered resource.
// Every other URI is a read of a resource template.
// Template variables are the numeric ids of that list or tree.
func (a *agent) listURIsLocked() []string {
	var uris []string
	add := func(u string) { uris = append(uris, u) }
	add("list://composition")
	for ci, w := range a.compositions() {
		cs := strconv.Itoa(ci)
		add("tree://node/" + cs)
		nodes := nodeTree(w)
		assignIDs(&nodes)
		for id, place := range indexTree(nodes, true) {
			if len(place.names) == 0 {
				continue
			}
			np := formatNodePath(place.names)
			ns := strconv.Itoa(id)
			add("tree://chunk/" + cs + "/" + ns)
			add("tree://item/" + cs + "/" + ns)
			add("tree://file/" + cs + "/" + ns)
			add("blocks://node/" + cs + "/" + ns)
			chain, err := w.Chain(np)
			if err != nil {
				continue
			}
			maps, err := w.Mappings(np)
			if err != nil {
				continue
			}
			chunks, err := chunkTree(chain, maps)
			if err != nil {
				continue
			}
			assignIDs(&chunks)
			for cid, cp := range indexTree(chunks, false) {
				if !cp.leaf {
					continue
				}
				ch := strconv.Itoa(cid)
				add("tree://specialization/" + cs + "/" + ns + "/" + ch)
				spec, err := specializationTree(chain, maps, chunkRefOf(cp))
				if err != nil {
					continue
				}
				assignIDs(&spec)
				for sid := range indexTree(spec, false) {
					ss := strconv.Itoa(sid)
					add("blocks://apply/" + cs + "/" + ns + "/" + ch + "/" + ss)
					add("blocks://effective/" + cs + "/" + ns + "/" + ch + "/" + ss)
					add("blocks://depend/" + cs + "/" + ns + "/" + ch + "/" + ss)
				}
			}
			items, err := itemTree(chain, maps)
			if err == nil {
				assignIDs(&items)
				for iid, ip := range indexTree(items, false) {
					if ip.leaf {
						add("blocks://refer/" + cs + "/" + ns + "/" + strconv.Itoa(iid))
					}
				}
			}
			files := fileTree(chain, maps)
			assignIDs(&files)
			for fid, fp := range indexTree(files, false) {
				if fp.leaf {
					add("file://" + cs + "/" + ns + "/" + strconv.Itoa(fid))
				}
			}
		}
	}
	return uris
}

func (a *agent) render(uri string) (text, mime string, blob []byte, err error) {
	mime = mimeOf(uri)
	switch {
	case uri == "list://composition":
		raw, err := json.Marshal(a.compositionList())
		if err != nil {
			return "", "", nil, err
		}
		return string(raw), mime, nil, nil
	case strings.HasPrefix(uri, "tree://node/"):
		w, _, err := a.idParts(strings.TrimPrefix(uri, "tree://node/"), 1)
		if err != nil {
			return "", "", nil, err
		}
		tree := nodeTree(w)
		assignIDs(&tree)
		raw, err := json.Marshal(tree)
		if err != nil {
			return "", "", nil, err
		}
		return string(raw), mime, nil, nil
	case strings.HasPrefix(uri, "tree://chunk/"):
		return a.renderTree(uri, "tree://chunk/", chunkTree)
	case strings.HasPrefix(uri, "tree://item/"):
		return a.renderTree(uri, "tree://item/", itemTree)
	case strings.HasPrefix(uri, "tree://file/"):
		return a.renderTree(uri, "tree://file/", func(chain []Block, maps []Mapping) (treeJSON, error) {
			return fileTree(chain, maps), nil
		})
	case strings.HasPrefix(uri, "tree://specialization/"):
		w, ids, err := a.idParts(strings.TrimPrefix(uri, "tree://specialization/"), 3)
		if err != nil {
			return "", "", nil, err
		}
		chain, maps, err := a.nodeChain(w, ids[1])
		if err != nil {
			return "", "", nil, err
		}
		chunk, err := chunkByID(chain, maps, ids[2])
		if err != nil {
			return "", "", nil, err
		}
		tree, err := specializationTree(chain, maps, chunk)
		if err != nil {
			return "", "", nil, err
		}
		assignIDs(&tree)
		raw, err := json.Marshal(tree)
		if err != nil {
			return "", "", nil, err
		}
		return string(raw), mime, nil, nil
	case strings.HasPrefix(uri, "blocks://node/"):
		w, ids, err := a.idParts(strings.TrimPrefix(uri, "blocks://node/"), 2)
		if err != nil {
			return "", "", nil, err
		}
		n, err := nodeByID(w, ids[1])
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

func (a *agent) compositionList() []listJSON {
	comps := a.compositions()
	out := make([]listJSON, 0, len(comps))
	for i, w := range comps {
		out = append(out, listJSON{
			Name:        w.Dir,
			Description: w.Description,
			ID:          i,
		})
	}
	return out
}

// idParts splits a URI tail of n decimal ids and resolves the composition.
// The caller holds a.mu.
func (a *agent) idParts(rest string, n int) (*World, []int, error) {
	segs := strings.Split(rest, "/")
	if rest == "" || len(segs) != n {
		return nil, nil, errString("unknown resource")
	}
	ids := make([]int, n)
	for i, s := range segs {
		id, err := strconv.Atoi(s)
		if err != nil || id < 0 || strconv.Itoa(id) != s {
			return nil, nil, errString("unknown resource")
		}
		ids[i] = id
	}
	w, err := a.compositionByID(ids[0])
	if err != nil {
		return nil, nil, err
	}
	return w, ids, nil
}

// nodeChain loads the block chain and mappings of a node id.
// The caller holds a.mu.
func (a *agent) nodeChain(w *World, id int) ([]Block, []Mapping, error) {
	n, err := nodeByID(w, id)
	if err != nil {
		return nil, nil, err
	}
	np := formatNodePath(nodePath(*n))
	chain, err := w.Chain(np)
	if err != nil {
		return nil, nil, err
	}
	maps, err := w.Mappings(np)
	if err != nil {
		return nil, nil, err
	}
	return chain, maps, nil
}

func (a *agent) renderTree(uri, prefix string, build func([]Block, []Mapping) (treeJSON, error)) (string, string, []byte, error) {
	w, ids, err := a.idParts(strings.TrimPrefix(uri, prefix), 2)
	if err != nil {
		return "", "", nil, err
	}
	chain, maps, err := a.nodeChain(w, ids[1])
	if err != nil {
		return "", "", nil, err
	}
	tree, err := build(chain, maps)
	if err != nil {
		return "", "", nil, err
	}
	assignIDs(&tree)
	raw, err := json.Marshal(tree)
	if err != nil {
		return "", "", nil, err
	}
	return string(raw), mimeOf(uri), nil, nil
}

func (a *agent) renderBlocks(uri, mime string) (string, string, []byte, error) {
	var kind, rest string
	n := 4
	switch {
	case strings.HasPrefix(uri, "blocks://apply/"):
		kind, rest = "literal", strings.TrimPrefix(uri, "blocks://apply/")
	case strings.HasPrefix(uri, "blocks://effective/"):
		kind, rest = "effective", strings.TrimPrefix(uri, "blocks://effective/")
	case strings.HasPrefix(uri, "blocks://depend/"):
		kind, rest = "depend", strings.TrimPrefix(uri, "blocks://depend/")
	default:
		kind, rest = "reference", strings.TrimPrefix(uri, "blocks://refer/")
		n = 3
	}
	w, ids, err := a.idParts(rest, n)
	if err != nil {
		return "", "", nil, err
	}
	node, err := nodeByID(w, ids[1])
	if err != nil {
		return "", "", nil, err
	}
	chain, maps, err := a.nodeChain(w, ids[1])
	if err != nil {
		return "", "", nil, err
	}
	var arg string
	if n == 4 {
		ref, err := chunkByID(chain, maps, ids[2])
		if err != nil {
			return "", "", nil, err
		}
		labels, err := specLabels(chain, maps, ref, ids[3])
		if err != nil {
			return "", "", nil, err
		}
		ref.Labels = labels
		arg = ref.String()
	} else {
		item, err := itemByID(chain, maps, ids[2])
		if err != nil {
			return "", "", nil, err
		}
		arg = formatItem(item)
	}
	text, err := dispatch(w, formatNodePath(nodePath(*node)), kind, arg, nil)
	if err != nil {
		return "", "", nil, err
	}
	return text, mime, nil, nil
}

// renderFile is called with a.mu held, and releases it while the file is evaluated.
func (a *agent) renderFile(uri, mime string) (string, string, []byte, error) {
	w, ids, err := a.idParts(strings.TrimPrefix(uri, "file://"), 3)
	if err != nil {
		return "", "", nil, err
	}
	chain, maps, err := a.nodeChain(w, ids[1])
	if err != nil {
		return "", "", nil, err
	}
	path, err := fileByID(chain, maps, ids[2])
	if err != nil {
		return "", "", nil, err
	}
	f := buildCatalog(chain, maps).files[path]
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
		a.mu.Unlock()
		text, err := evalReporting(chain, maps, ChunkRef{Chunk: Name(path)}, nil)
		a.mu.Lock()
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
			t.SubTrees = append(t.SubTrees, treeJSON{
				Name:        e.name,
				Description: d,
				SubTrees:    emptySub(),
				Leaf:        true,
			})
		}
	}
	return t
}

func newAgent() *agent {
	a := &agent{
		worlds: map[string]*World{},
		uris:   []string{"list://composition"},
	}
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
		Name:        "import_composition",
		Description: "Import an existing composition.",
	}, a.importComposition)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "create_composition",
		Description: "Create a new composition, and import it. Specify a ground, or leave the ground as null.",
		InputSchema: nullableStringSchema[createArchiveIn]("ground"),
	}, a.createComposition)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "create_node",
		Description: "Create a new node. The corresponding archive file is created empty and must not already exist. A null base creates the root.",
		InputSchema: nullableIntegerSchema[createIn]("base"),
	}, a.createNode)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "delete_node",
		Description: "Delete a node and every node based on it, and delete their archive files.",
	}, a.deleteNode)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "delete_chunk",
		Description: "Delete a chunk. All applied blocks are deleted.",
	}, a.deleteChunk)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "delete_file",
		Description: "Delete a file. All applied blocks are deleted.",
	}, a.deleteFile)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "evaluate_expression",
		Description: "Evaluate the content of an expression.",
	}, a.evaluate)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "apply_block",
		Description: "Apply a block to a node. Cannot apply a block to a node in the ground archive.",
	}, a.applyBlock)
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
	return a
}

// nullableStringSchema is the inferred object schema with field rewritten.
// A *string infers "type": ["null", "string"]. Some clients accept only
// one type per schema, so null and string are separate anyOf branches.
func nullableStringSchema[T any](field string) *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	prop := s.Properties[field]
	s.Properties[field] = &jsonschema.Schema{
		Description: prop.Description,
		AnyOf: []*jsonschema.Schema{
			{Type: "null"},
			{Type: "string"},
		},
	}
	return s
}

// nullableIntegerSchema publishes field as null or an integer.
// A *int infers "type": ["null", "integer"]. Some clients accept only
// one type per schema, so null and integer are separate anyOf branches.
func nullableIntegerSchema[T any](field string) *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	prop := s.Properties[field]
	s.Properties[field] = &jsonschema.Schema{
		Description: prop.Description,
		AnyOf: []*jsonschema.Schema{
			{Type: "null"},
			{Type: "integer"},
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

// evalReporting runs without a.mu, since a filter may run for long;
// chain and maps are copies taken under it.
// A failed evaluation carries the stderr of its filters,
// which is the only report a client sees.
func evalReporting(chain []Block, maps []Mapping, ref ChunkRef, filters []Filter) (string, error) {
	var stderr bytes.Buffer
	ev := NewEvaluator(chain, maps, &stderr)
	text, err := ev.Eval(ref, filters)
	if err != nil {
		if msg := strings.TrimRight(stderr.String(), "\n"); msg != "" {
			return "", fmt.Errorf("%w\n%s", err, msg)
		}
		return "", err
	}
	return text, nil
}

func mimeOf(uri string) string {
	if strings.HasPrefix(uri, "tree:") || strings.HasPrefix(uri, "list:") {
		return "application/json"
	}
	return "text/plain"
}

func readOrigin(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errString("could not read '" + path + "'")
	}
	return data, nil
}

func emptySub() []treeJSON {
	return []treeJSON{}
}

func newNest() *nest {
	return &nest{leaves: map[string]bool{}, subs: map[string]*nest{}}
}

// A reserved chunk lives in the Module, Chunk, Specialization, Domain, or Index namespace.
// Its module path starts with that name. The remaining components, then the
// chunk name, are the module, chunk, specialization, domain, or item the chunk describes.
// A specialization path is stored reversed: label2@:label1 is Specialization@/label1@/label2.
func reservedNamespace(mod ModulePath) (Name, bool) {
	if len(mod) == 0 {
		return "", false
	}
	switch mod[0] {
	case "Module", "Chunk", "Specialization", "Domain", "Index":
		return mod[0], true
	default:
		return "", false
	}
}

func newMeta() metaInfo {
	return metaInfo{
		modules: map[string]string{},
		chunks:  map[string]string{},
		domains: map[string]string{},
		items:   map[string]string{},
		specs:   map[string]string{},
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
		var desc string
		text, err := ev.Eval(ref, nil)
		if err != nil {
			return metaInfo{}, err
		}
		desc, err = metaDescription(text)
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
		case "Specialization":
			meta.specs[specName(target)] = desc
		}
	}
	return meta, nil
}

func specName(reversed []Name) string {
	parts := make([]string, len(reversed))
	for i, n := range reversed {
		parts[len(reversed)-1-i] = string(n)
	}
	return strings.Join(parts, "@:")
}

func labelsText(ls []Name) string {
	parts := make([]string, len(ls))
	for i, n := range ls {
		parts[i] = string(n)
	}
	return strings.Join(parts, "@:")
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
	return root.treeWith("/", meta.modules[pathKey(nil)], nil,
		func(path []Name) string { return meta.modules[pathKey(path)] },
		func(path []Name, name string) string { return meta.chunks[leafKey(path, name)] },
	), nil
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
	return root.treeWith("/", "", nil, nil, nil)
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
	return root.treeWith("/", meta.domains[pathKey(nil)], nil,
		func(path []Name) string { return meta.domains[pathKey(path)] },
		func(path []Name, name string) string { return meta.items[leafKey(path, name)] },
	), nil
}

func nodeTree(w *World) treeJSON {
	children := map[string][]*Node{}
	var root *Node
	for i := range w.Nodes {
		n := &w.Nodes[i]
		if len(n.Base) == 0 {
			root = n
			continue
		}
		children[formatNodePath(n.Base)] = append(children[formatNodePath(n.Base)], n)
	}
	if root == nil {
		return treeJSON{SubTrees: emptySub()}
	}
	var build func(*Node) treeJSON
	build = func(n *Node) treeJSON {
		path := formatNodePath(nodePath(*n))
		t := treeJSON{
			Name:        n.Name,
			Description: n.Description,
			SubTrees:    emptySub(),
			Leaf:        true,
		}
		for _, c := range children[path] {
			t.SubTrees = append(t.SubTrees, build(c))
		}
		return t
	}
	return build(root)
}

// specializationTree is the specializations of one chunk.
// The root is named "/" and is the unspecialized form.
// Each child is one label, general toward specific.
// The path label1/label2 is the specialization label2@:label1.
// Every node, including the root, is a specialization.
// A specialization is a label path on a block of the chunk,
// or on a reference to the chunk.
func specializationTree(blocks []Block, maps []Mapping, chunk ChunkRef) (treeJSON, error) {
	meta, err := describe(blocks, maps)
	if err != nil {
		return treeJSON{}, err
	}
	type specNode struct {
		name  string
		path  []string
		kids  []*specNode
		index map[string]*specNode
	}
	root := &specNode{name: "/", index: map[string]*specNode{}}
	ensure := func(labels []Name) {
		cur := root
		for i := len(labels) - 1; i >= 0; i-- {
			name := string(labels[i])
			child := cur.index[name]
			if child == nil {
				path := append(append([]string{}, cur.path...), name)
				child = &specNode{name: name, path: path, index: map[string]*specNode{}}
				cur.index[name] = child
				cur.kids = append(cur.kids, child)
			}
			cur = child
		}
	}
	note := func(ref ChunkRef) {
		if ref.Chunk == chunk.Chunk && modulesEqual(ref.Module, chunk.Module) {
			ensure(ref.Labels)
		}
	}
	for _, b := range blocks {
		note(b.Ref)
		for _, part := range b.Parts {
			if part.Ref == nil {
				continue
			}
			resolved := resolveBody(part.Ref.ChunkRef, b.Ref.Labels, b.Ref.Module)
			note(resolved)
			for _, f := range part.Ref.Filters {
				note(resolveFilter(f.Ref, resolved.Labels))
			}
		}
		for _, f := range b.Filters {
			note(resolveFilter(f.Ref, b.Ref.Labels))
		}
	}
	var build func(*specNode) treeJSON
	build = func(n *specNode) treeJSON {
		key := ""
		if len(n.path) > 0 {
			rev := make([]string, len(n.path))
			for i, part := range n.path {
				rev[len(n.path)-1-i] = part
			}
			key = strings.Join(rev, "@:")
		}
		t := treeJSON{
			Name:        n.name,
			Description: meta.specs[key],
			SubTrees:    emptySub(),
			Leaf:        true,
		}
		for _, c := range n.kids {
			t.SubTrees = append(t.SubTrees, build(c))
		}
		return t
	}
	tree := build(root)
	assignIDs(&tree)
	return tree, nil
}

func assignIDs(t *treeJSON) {
	n := 0
	var walk func(*treeJSON)
	walk = func(t *treeJSON) {
		t.ID = n
		n++
		for i := range t.SubTrees {
			walk(&t.SubTrees[i])
		}
	}
	walk(t)
}

// indexTree maps each id to its path.
// includeRoot puts the root object's name on that path.
// Chunk, file, item, and specialization trees leave it off,
// so the wrapper named "/" does not appear in the path.
// The wrapper's own id is still present, with an empty path.
func indexTree(t treeJSON, includeRoot bool) map[int]treePlace {
	out := map[int]treePlace{}
	var walk func(treeJSON, []string)
	walk = func(node treeJSON, path []string) {
		out[node.ID] = treePlace{names: append([]string{}, path...), leaf: node.Leaf}
		for _, sub := range node.SubTrees {
			next := append(append([]string{}, path...), sub.Name)
			walk(sub, next)
		}
	}
	var start []string
	if includeRoot && t.Name != "" {
		start = []string{t.Name}
	}
	walk(t, start)
	return out
}

func nodeByID(w *World, id int) (*Node, error) {
	tree := nodeTree(w)
	assignIDs(&tree)
	place, ok := indexTree(tree, true)[id]
	if !ok || len(place.names) == 0 {
		return nil, fmt.Errorf("unknown node %d", id)
	}
	i, err := w.findIndex(formatNodePath(place.names))
	if err != nil {
		return nil, fmt.Errorf("unknown node %d", id)
	}
	return &w.Nodes[i], nil
}

func nodeID(w *World, path string) (int, error) {
	tree := nodeTree(w)
	assignIDs(&tree)
	for id, place := range indexTree(tree, true) {
		if formatNodePath(place.names) == path {
			return id, nil
		}
	}
	return 0, fmt.Errorf("unknown node '%s'", path)
}

func chunkRefOf(p treePlace) ChunkRef {
	if len(p.names) == 0 {
		return ChunkRef{}
	}
	mod := make(ModulePath, len(p.names)-1)
	for i, name := range p.names[:len(p.names)-1] {
		mod[i] = Name(name)
	}
	return ChunkRef{Module: mod, Chunk: Name(p.names[len(p.names)-1])}
}

func chunkByID(blocks []Block, maps []Mapping, id int) (ChunkRef, error) {
	tree, err := chunkTree(blocks, maps)
	if err != nil {
		return ChunkRef{}, err
	}
	assignIDs(&tree)
	place, ok := indexTree(tree, false)[id]
	if !ok || !place.leaf {
		return ChunkRef{}, fmt.Errorf("unknown chunk %d", id)
	}
	return chunkRefOf(place), nil
}

func fileByID(blocks []Block, maps []Mapping, id int) (string, error) {
	tree := fileTree(blocks, maps)
	assignIDs(&tree)
	place, ok := indexTree(tree, false)[id]
	if !ok || !place.leaf {
		return "", fmt.Errorf("unknown file %d", id)
	}
	return strings.Join(place.names, "/"), nil
}

func itemByID(blocks []Block, maps []Mapping, id int) ([]Name, error) {
	tree, err := itemTree(blocks, maps)
	if err != nil {
		return nil, err
	}
	assignIDs(&tree)
	place, ok := indexTree(tree, false)[id]
	if !ok || !place.leaf {
		return nil, fmt.Errorf("unknown item %d", id)
	}
	item := make([]Name, len(place.names))
	for i, name := range place.names {
		item[i] = Name(name)
	}
	return item, nil
}

func specLabels(blocks []Block, maps []Mapping, chunk ChunkRef, id int) ([]Name, error) {
	tree, err := specializationTree(blocks, maps, chunk)
	if err != nil {
		return nil, err
	}
	place, ok := indexTree(tree, false)[id]
	if !ok {
		return nil, fmt.Errorf("unknown specialization %d", id)
	}
	labels := make([]Name, len(place.names))
	for i, name := range place.names {
		labels[len(place.names)-1-i] = Name(name)
	}
	return labels, nil
}
