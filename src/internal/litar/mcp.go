package litar

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// searchLimit is the largest number of blocks a search answers.
const searchLimit = 100

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
	{"node blocks", "the blocks in the node", "blocks://node/{composition}/{node}", "application/json"},
	{"specializations", "the specialization tree under the node", "tree://specialization/{composition}/{node}", "application/json"},
	{"applied blocks", "the blocks applied to the chunk reference", "blocks://apply/{composition}/{node}/{chunk}/{specialization}", "application/json"},
	{"effective blocks", "the effective blocks of the chunk reference", "blocks://effective/{composition}/{node}/{chunk}/{specialization}", "application/json"},
	{"dependent blocks", "the blocks depending on a chunk reference", "blocks://depend/{composition}/{node}/{chunk}/{specialization}", "application/json"},
	{"related blocks", "the blocks referring to the item", "blocks://refer/{composition}/{node}/{item}", "application/json"},
	{"derived blocks", "the blocks written under exactly the label path", "blocks://specialization/{composition}/{node}/{labels}", "application/json"},
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
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Name        string `json:"name" jsonschema:"node name"`
	Archive     string `json:"archive" jsonschema:"archive path; the file is created empty"`
	Description string `json:"description" jsonschema:"node description"`
	// Null creates the root.
	Base     *string     `json:"base" jsonschema:"ref of the parent node, or null to create the root"`
	Mappings []mappingIn `json:"mappings,omitempty" jsonschema:"files mapped into the node"`
}

type deleteNodeIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node" jsonschema:"ref of the node"`
}

type deleteChunkIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node" jsonschema:"ref of the node"`
	Chunk       string `json:"chunk" jsonschema:"ref of the chunk"`
}

type deleteFileIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node" jsonschema:"ref of the node"`
	File        string `json:"file" jsonschema:"ref of the file"`
}

type evalIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node" jsonschema:"ref of the node"`
	Expression  string `json:"expression" jsonschema:"expression to evaluate"`
}

type applyIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node" jsonschema:"ref of the node"`
	Text        string `json:"text" jsonschema:"block text, with its comment before it"`
	Overrides   string `json:"overrides,omitempty" jsonschema:"ref of a block of the node applied to the same chunk reference; the new block takes its place"`
}

type errString string

type listJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Ref         string `json:"ref"`
}

type treeJSON struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Ref         string     `json:"ref"`
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
	chunks map[string]string
	specs  map[string]string
	files  map[string]string
	items  map[string]string
}

type treePlace struct {
	names []string
	leaf  bool
}

type renameIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node" jsonschema:"ref of the node"`
	Object      string `json:"object" jsonschema:"ref of a chunk or module of the chunk tree, or of an item or domain of the item tree"`
	To          string `json:"to" jsonschema:"new path, such as Module@/chunk"`
}

type filesIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node" jsonschema:"ref of the node"`
	Command     string `json:"command" jsonschema:"one rm or mv command, such as mv src/a.c lib/"`
}

type mappingIn struct {
	Path    string   `json:"path" jsonschema:"path in the node; a directory ends with /"`
	Origin  string   `json:"origin" jsonschema:"path of the source, relative to the composition directory"`
	FileSet []string `json:"fileset,omitempty" jsonschema:"file sets the mapped files join"`
}

type mapIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node" jsonschema:"ref of the node"`
	mappingIn
}

type blockIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node" jsonschema:"ref of the node"`
	Block       string `json:"block" jsonschema:"ref of a block of the node"`
}

type moveIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node" jsonschema:"ref of the node"`
	Block       string `json:"block" jsonschema:"ref of a block of the node"`
	To          string `json:"to" jsonschema:"ref of the node that receives the block"`
}

// A blockJSON is one block of a blocks:// resource.
// Node is left out when the block belongs to the node of the resource.
type blockJSON struct {
	Ref     string `json:"ref"`
	Node    string `json:"node,omitempty"`
	Comment string `json:"comment,omitempty"`
	Text    string `json:"text"`
}

type describeIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node,omitempty" jsonschema:"ref of the node; leave it out to describe the composition"`
	Description string `json:"description" jsonschema:"new description"`
}

type searchIn struct {
	Composition string `json:"composition" jsonschema:"ref of the composition"`
	Node        string `json:"node" jsonschema:"ref of the node"`
	Text        string `json:"text" jsonschema:"string to find in the comments and texts of the blocks"`
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
	a.addWorld(w)
	a.refresh()
	return toolText(compositionRef(w))
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
	ref, err := a.createNodeLocked(in)
	a.mu.Unlock()
	if err != nil {
		return toolErr(err)
	}
	a.refresh()
	return toolText(ref)
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
	return toolText(in.Node)
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
	ref, err := chunkByRef(chain, maps, in.Chunk)
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
	return toolText(in.Chunk)
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
	path, err := fileByRef(chain, maps, in.File)
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
	return toolText(in.File)
}

func (a *agent) evaluate(ctx context.Context, _ *mcp.CallToolRequest, in evalIn) (*mcp.CallToolResult, any, error) {
	ref, filters, err := ParsePrint(in.Expression)
	if err != nil {
		return toolErr(err)
	}
	a.mu.Lock()
	w, err := a.compositionByRef(in.Composition)
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

// applyBlock answers with the refs of the blocks of the text, one per line,
// so that a later call can address them without reading the node again.
func (a *agent) applyBlock(ctx context.Context, _ *mcp.CallToolRequest, in applyIn) (*mcp.CallToolResult, any, error) {
	arch, err := ParseArchive([]byte(in.Text), "")
	if err != nil {
		return toolErr(err)
	}
	a.mu.Lock()
	w, n, err := a.worldNode(in.Composition, in.Node)
	switch {
	case err != nil:
	case in.Overrides == "":
		err = w.ApplyBlock(formatNodePath(nodePath(*n)), in.Text)
	default:
		err = w.OverrideBlock(formatNodePath(nodePath(*n)), in.Overrides, in.Text)
	}
	a.mu.Unlock()
	if err != nil {
		return toolErr(err)
	}
	a.refresh()
	refs := make([]string, len(arch.Blocks))
	for i, b := range arch.Blocks {
		refs[i] = blockRef(b)
	}
	return toolText(strings.Join(refs, "\n"))
}

// worldNode resolves a composition ref and a node ref.
// The caller holds a.mu.
func (a *agent) worldNode(composition, node string) (*World, *Node, error) {
	w, err := a.compositionByRef(composition)
	if err != nil {
		return nil, nil, err
	}
	n, err := nodeByRef(w, node)
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
// Template variables are the refs of that list or tree.
func (a *agent) listURIsLocked() []string {
	uris := []string{"list://composition"}
	for _, w := range a.compositions() {
		cs := compositionRef(w)
		uris = append(uris, "tree://node/"+cs)
		for i := range w.Nodes {
			uris = append(uris, nodeURIs(w, cs, &w.Nodes[i])...)
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
		w, _, err := a.refParts(strings.TrimPrefix(uri, "tree://node/"), 1)
		if err != nil {
			return "", "", nil, err
		}
		raw, err := json.Marshal(nodeTree(w))
		if err != nil {
			return "", "", nil, err
		}
		return string(raw), mime, nil, nil
	case strings.HasPrefix(uri, "tree://chunk/"):
		return a.renderTree(uri, "tree://chunk/", chunkTree)
	case strings.HasPrefix(uri, "tree://item/"):
		return a.renderTree(uri, "tree://item/", itemTree)
	case strings.HasPrefix(uri, "tree://file/"):
		return a.renderTree(uri, "tree://file/", fileTree)
	case strings.HasPrefix(uri, "tree://specialization/"):
		return a.renderTree(uri, "tree://specialization/", labelTree)
	case strings.HasPrefix(uri, "blocks://"):
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
	for _, w := range comps {
		out = append(out, listJSON{
			Name:        w.Dir,
			Description: w.Description,
			Ref:         compositionRef(w),
		})
	}
	return out
}

// nodeChain loads the block chain and mappings of a node ref.
// The caller holds a.mu.
func (a *agent) nodeChain(w *World, ref string) ([]Block, []Mapping, error) {
	n, err := nodeByRef(w, ref)
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
	w, refs, err := a.refParts(strings.TrimPrefix(uri, prefix), 2)
	if err != nil {
		return "", "", nil, err
	}
	chain, maps, err := a.nodeChain(w, refs[1])
	if err != nil {
		return "", "", nil, err
	}
	tree, err := build(chain, maps)
	if err != nil {
		return "", "", nil, err
	}
	raw, err := json.Marshal(tree)
	if err != nil {
		return "", "", nil, err
	}
	return string(raw), mimeOf(uri), nil, nil
}

// renderBlocks answers a blocks:// resource: the kind after the scheme says
// which blocks of the node's chain are listed, and how many refs follow.
func (a *agent) renderBlocks(uri, mime string) (string, string, []byte, error) {
	kind, rest, _ := strings.Cut(strings.TrimPrefix(uri, "blocks://"), "/")
	count := map[string]int{"node": 2, "refer": 3, "specialization": 3, "apply": 4, "effective": 4, "depend": 4}[kind]
	w, refs, err := a.refParts(rest, count)
	if err != nil {
		return "", "", nil, err
	}
	node, err := nodeByRef(w, refs[1])
	if err != nil {
		return "", "", nil, err
	}
	chain, maps, err := a.nodeChain(w, refs[1])
	if err != nil {
		return "", "", nil, err
	}
	blocks := node.Own
	if kind != "node" {
		if blocks, err = selectBlocks(kind, chain, maps, refs[2:]); err != nil {
			return "", "", nil, err
		}
	}
	raw, err := json.Marshal(blockList(w, node, blocks))
	if err != nil {
		return "", "", nil, err
	}
	return string(raw), mime, nil, nil
}

// renderFile is called with a.mu held, and releases it while the file is evaluated.
func (a *agent) renderFile(uri, mime string) (string, string, []byte, error) {
	w, refs, err := a.refParts(strings.TrimPrefix(uri, "file://"), 3)
	if err != nil {
		return "", "", nil, err
	}
	chain, maps, err := a.nodeChain(w, refs[1])
	if err != nil {
		return "", "", nil, err
	}
	path, err := fileByRef(chain, maps, refs[2])
	if err != nil {
		return "", "", nil, err
	}
	f := buildCatalog(chain, maps).file(path)
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

// chunkDescription describes the module or chunk at path in the chunk tree.
// Below File, the path names a file or directory.
func (m metaInfo) chunkDescription(path []Name) string {
	if len(path) > 0 && path[0] == "File" {
		return m.files[pathKey(path[1:])]
	}
	return m.chunks[pathKey(path)]
}

// rename takes the ref of an entry of the chunk tree or of the item tree.
// A branch and a leaf have different refs, so the ref alone says what is renamed.
func (a *agent) rename(ctx context.Context, _ *mcp.CallToolRequest, in renameIn) (*mcp.CallToolResult, any, error) {
	to, err := ParseModulePath(in.To)
	if err != nil {
		return toolErr(err)
	}
	a.mu.Lock()
	from, err := a.renameLocked(in, to)
	a.mu.Unlock()
	if err != nil {
		return toolErr(err)
	}
	a.refresh()
	return toolText(from + " -> " + in.To)
}

func (a *agent) operateFiles(ctx context.Context, _ *mcp.CallToolRequest, in filesIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	w, n, err := a.worldNode(in.Composition, in.Node)
	var report string
	if err == nil {
		report, err = w.OperateFiles(formatNodePath(nodePath(*n)), in.Command)
	}
	a.mu.Unlock()
	if err != nil {
		return toolErr(err)
	}
	a.refresh()
	return toolText(report)
}

// compositionByRef resolves a ref from list://composition.
// The caller holds a.mu.
func (a *agent) compositionByRef(ref string) (*World, error) {
	for _, w := range a.compositions() {
		if compositionRef(w) == ref {
			return w, nil
		}
	}
	return nil, fmt.Errorf("unknown composition '%s'", ref)
}

// createNodeLocked creates the node with its mappings and returns its ref.
// A mapping that fails takes the new node away again.
func (a *agent) createNodeLocked(in createIn) (string, error) {
	w, err := a.compositionByRef(in.Composition)
	if err != nil {
		return "", err
	}
	var base []string
	if in.Base != nil {
		parent, err := nodeByRef(w, *in.Base)
		if err != nil {
			return "", err
		}
		base = nodePath(*parent)
	}
	if err := w.CreateNode(in.Name, in.Archive, in.Description, base); err != nil {
		return "", err
	}
	path := append(base, in.Name)
	for _, m := range in.Mappings {
		if err := w.MapFile(formatNodePath(path), m.Path, m.Origin, m.FileSet); err != nil {
			_ = w.DeleteNode(formatNodePath(path))
			return "", err
		}
	}
	return refOf("node", path...), nil
}

func (a *agent) mapFile(ctx context.Context, _ *mcp.CallToolRequest, in mapIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	w, n, err := a.worldNode(in.Composition, in.Node)
	if err == nil {
		err = w.MapFile(formatNodePath(nodePath(*n)), in.Path, in.Origin, in.FileSet)
	}
	a.mu.Unlock()
	if err != nil {
		return toolErr(err)
	}
	a.refresh()
	return toolText(in.Path)
}

func (a *agent) deleteBlock(ctx context.Context, _ *mcp.CallToolRequest, in blockIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	w, n, err := a.worldNode(in.Composition, in.Node)
	if err == nil {
		err = w.DeleteBlock(formatNodePath(nodePath(*n)), in.Block)
	}
	a.mu.Unlock()
	if err != nil {
		return toolErr(err)
	}
	a.refresh()
	return toolText(in.Block)
}

func (a *agent) moveBlock(ctx context.Context, _ *mcp.CallToolRequest, in moveIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	w, n, err := a.worldNode(in.Composition, in.Node)
	var to *Node
	if err == nil {
		to, err = nodeByRef(w, in.To)
	}
	if err == nil {
		err = w.MoveBlock(formatNodePath(nodePath(*n)), in.Block, formatNodePath(nodePath(*to)))
	}
	a.mu.Unlock()
	if err != nil {
		return toolErr(err)
	}
	a.refresh()
	return toolText(in.Block)
}

// refParts splits a URI tail of n refs and resolves the composition.
// The caller holds a.mu.
func (a *agent) refParts(rest string, n int) (*World, []string, error) {
	refs := strings.Split(rest, "/")
	if rest == "" || len(refs) != n {
		return nil, nil, errString("unknown resource")
	}
	w, err := a.compositionByRef(refs[0])
	if err != nil {
		return nil, nil, err
	}
	return w, refs, nil
}

func (a *agent) renameLocked(in renameIn, to ModulePath) (string, error) {
	w, n, err := a.worldNode(in.Composition, in.Node)
	if err != nil {
		return "", err
	}
	chain, maps, err := a.nodeChain(w, in.Node)
	if err != nil {
		return "", err
	}
	r := renaming{to: to}
	for _, build := range []func([]Block, []Mapping) (treeJSON, error){chunkTree, itemTree} {
		tree, err := build(chain, maps)
		if err != nil {
			return "", err
		}
		if place, ok := indexTree(tree, false)[in.Object]; ok && len(place.names) > 0 {
			r.from, r.module = toNames(place.names), !place.leaf
			return formatModule(ModulePath(r.from)), w.Rename(formatNodePath(nodePath(*n)), r)
		}
		r.items = true
	}
	return "", fmt.Errorf("unknown object '%s'", in.Object)
}

func (a *agent) describe(ctx context.Context, _ *mcp.CallToolRequest, in describeIn) (*mcp.CallToolResult, any, error) {
	a.mu.Lock()
	w, err := a.compositionByRef(in.Composition)
	name, described := "", in.Composition
	if err == nil && in.Node != "" {
		var n *Node
		if n, err = nodeByRef(w, in.Node); err == nil {
			name, described = formatNodePath(nodePath(*n)), in.Node
		}
	}
	if err == nil {
		err = w.Describe(name, in.Description)
	}
	a.mu.Unlock()
	if err != nil {
		return toolErr(err)
	}
	a.refresh()
	return toolText(described)
}

func (a *agent) searchBlocks(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
	if in.Text == "" {
		return toolErr(errString("the text to search for is empty"))
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	w, n, err := a.worldNode(in.Composition, in.Node)
	if err != nil {
		return toolErr(err)
	}
	chain, _, err := a.nodeChain(w, in.Node)
	if err != nil {
		return toolErr(err)
	}
	got := []Block{}
	for _, b := range chain {
		if strings.Contains(b.Comment, in.Text) || strings.Contains(b.Raw, in.Text) {
			got = append(got, b)
		}
	}
	if len(got) > searchLimit {
		return toolErr(fmt.Errorf("%d blocks contain '%s'; search for a longer string", len(got), in.Text))
	}
	raw, err := json.Marshal(blockList(w, n, got))
	if err != nil {
		return toolErr(err)
	}
	return toolText(string(raw))
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
		Description: "Create a new node. The corresponding archive file is created empty and must not already exist. A null base creates the root. Mappings can be specified.",
		InputSchema: plainArrays(nullableStringSchema[createIn]("base")),
	}, a.createNode)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "map_file",
		Description: "Map a file or a directory outside the archives into a node. The rm of operate_files removes a mapping.",
		InputSchema: plainArrays(inputSchema[mapIn]()),
	}, a.mapFile)
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
		Description: "Apply a block to a node. Cannot apply a block to a node in the ground archive. With overrides, the block takes the place of a block applied to the same chunk reference. Returns the refs of the applied blocks.",
	}, a.applyBlock)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "delete_block",
		Description: "Delete a block of a node.",
	}, a.deleteBlock)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "move_block",
		Description: "Move a block of a node to another node of the composition.",
	}, a.moveBlock)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "rename",
		Description: "Rename a chunk, a module, an item, or a domain. Block headers, references, filters, items, and descriptions follow, in the node and the nodes based on it.",
	}, a.rename)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "operate_files",
		Description: "Operate files of a node with one shell command: rm [-r] [-f] PATH..., or mv SOURCE... TARGET.",
	}, a.operateFiles)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "describe",
		Description: "Replace the description of a node, or of the composition when no node is given.",
	}, a.describe)
	mcp.AddTool(a.server, &mcp.Tool{
		Name:        "search_blocks",
		Description: "List the blocks of a node and its bases whose comment or text contains a string, as a blocks:// resource does.",
	}, a.searchBlocks)
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
	if !strings.HasPrefix(uri, "file:") {
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

// A reserved chunk lives in the Chunk, Specialization, File, or Index namespace.
// Its module path starts with that name. The remaining components, then the
// chunk name, are the module or chunk, the specialization, the file or directory,
// or the domain or item the chunk describes.
// A specialization path is stored reversed: label2@:label1 is Specialization@/label1@/label2.
func reservedNamespace(mod ModulePath) (Name, bool) {
	if len(mod) == 0 {
		return "", false
	}
	switch mod[0] {
	case "Chunk", "Specialization", "File", "Index":
		return mod[0], true
	default:
		return "", false
	}
}

func newMeta() metaInfo {
	return metaInfo{
		chunks: map[string]string{},
		specs:  map[string]string{},
		files:  map[string]string{},
		items:  map[string]string{},
	}
}

func pathKey(path []Name) string {
	return formatModule(path)
}

func joinPath(path []Name, name string) []Name {
	return append(append([]Name{}, path...), Name(name))
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
		case "Chunk":
			meta.chunks[pathKey(target)] = desc
		case "Specialization":
			meta.specs[specName(target)] = desc
		case "File":
			meta.files[pathKey(target)] = desc
		case "Index":
			meta.items[pathKey(target)] = desc
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

func chunkTree(blocks []Block, maps []Mapping) (treeJSON, error) {
	meta, err := describe(blocks, maps)
	if err != nil {
		return treeJSON{}, err
	}
	root := newNest()
	files := map[Name]bool{}
	for _, b := range blocks {
		if b.IsFile {
			files[b.Path] = true
		}
	}
	for _, b := range blocks {
		if _, reserved := reservedNamespace(b.Ref.Module); reserved {
			continue
		}
		if len(b.Ref.Module) == 0 && files[b.Ref.Chunk] {
			cur := root.addSub("File")
			parts := strings.Split(string(b.Ref.Chunk), "/")
			for _, part := range parts[:len(parts)-1] {
				cur = cur.addSub(part)
			}
			cur.addLeaf(parts[len(parts)-1])
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
	tree := root.treeWith("/", meta.chunkDescription(nil), nil,
		meta.chunkDescription,
		func(path []Name, name string) string { return meta.chunkDescription(joinPath(path, name)) },
	)
	assignRefs(&tree, "module", "chunk", false)
	return tree, nil
}

func fileTree(blocks []Block, mappings []Mapping) (treeJSON, error) {
	meta, err := describe(blocks, mappings)
	if err != nil {
		return treeJSON{}, err
	}
	cat := buildCatalog(blocks, mappings)
	root := newNest()
	for _, path := range cat.order {
		parts := strings.Split(path, "/")
		cur := root
		for _, part := range parts[:len(parts)-1] {
			cur = cur.addSub(part)
		}
		last := parts[len(parts)-1]
		f := cat.files[path]
		if !f.dir || f.archive {
			cur.addLeaf(last)
			continue
		}
		dir := cur.addSub(last)
		_ = filepath.WalkDir(f.origin, func(p string, d os.DirEntry, err error) error {
			rel, relErr := filepath.Rel(f.origin, p)
			if err != nil || relErr != nil || rel == "." {
				return nil
			}
			names := strings.Split(filepath.ToSlash(rel), "/")
			at := dir
			for _, name := range names[:len(names)-1] {
				at = at.addSub(name)
			}
			if d.IsDir() {
				at.addSub(names[len(names)-1])
			} else {
				at.addLeaf(names[len(names)-1])
			}
			return nil
		})
	}
	tree := root.treeWith("/", meta.files[pathKey(nil)], nil,
		func(path []Name) string { return meta.files[pathKey(path)] },
		func(path []Name, name string) string { return meta.files[pathKey(joinPath(path, name))] },
	)
	assignRefs(&tree, "directory", "file", false)
	return tree, nil
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
	tree := root.treeWith("/", meta.items[pathKey(nil)], nil,
		func(path []Name) string { return meta.items[pathKey(path)] },
		func(path []Name, name string) string { return meta.items[pathKey(joinPath(path, name))] },
	)
	assignRefs(&tree, "domain", "item", false)
	return tree, nil
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
	tree := build(root)
	assignRefs(&tree, "node", "node", true)
	return tree
}

// specializationTree is the specializations of one chunk, or of every chunk
// when only is nil.
// The root is named "/" and is the unspecialized form.
// Each child is one label, general toward specific.
// The path label1/label2 is the specialization label2@:label1.
// Every node, including the root, is a specialization.
// A specialization is a label path on a block of the chunk,
// or on a reference to the chunk.
func specializationTree(blocks []Block, maps []Mapping, only *ChunkRef) (treeJSON, error) {
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
		if only == nil || ref.Chunk == only.Chunk && modulesEqual(ref.Module, only.Module) {
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
	assignRefs(&tree, "labels", "labels", false)
	return tree, nil
}

// indexTree maps each ref to its path.
// includeRoot puts the root object's name on that path.
// Chunk, file, item, and specialization trees leave it off,
// so the wrapper named "/" does not appear in the path.
// The wrapper's own ref is still present, with an empty path.
func indexTree(t treeJSON, includeRoot bool) map[string]treePlace {
	out := map[string]treePlace{}
	var walk func(treeJSON, []string)
	walk = func(node treeJSON, path []string) {
		out[node.Ref] = treePlace{names: append([]string{}, path...), leaf: node.Leaf}
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

// A leaf below File is the file chunk of the path under File.
func chunkRefOf(p treePlace) ChunkRef {
	if len(p.names) == 0 {
		return ChunkRef{}
	}
	if len(p.names) > 1 && p.names[0] == "File" {
		return ChunkRef{Chunk: Name(strings.Join(p.names[1:], "/"))}
	}
	mod := make(ModulePath, len(p.names)-1)
	for i, name := range p.names[:len(p.names)-1] {
		mod[i] = Name(name)
	}
	return ChunkRef{Module: mod, Chunk: Name(p.names[len(p.names)-1])}
}

// nodeURIs lists the URIs below one node. A chunk is paired only with the
// label paths it is used with, although any label path of the node is accepted.
func nodeURIs(w *World, cs string, n *Node) []string {
	tail := cs + "/" + nodeRef(*n)
	uris := []string{
		"tree://chunk/" + tail, "tree://item/" + tail, "tree://file/" + tail,
		"tree://specialization/" + tail, "blocks://node/" + tail,
	}
	np := formatNodePath(nodePath(*n))
	chain, err := w.Chain(np)
	if err != nil {
		return uris
	}
	maps, err := w.Mappings(np)
	if err != nil {
		return uris
	}
	leaves := func(build func([]Block, []Mapping) (treeJSON, error), each func(string, treePlace)) {
		tree, err := build(chain, maps)
		if err != nil {
			return
		}
		for ref, place := range indexTree(tree, false) {
			if place.leaf {
				each(ref, place)
			}
		}
	}
	leaves(chunkTree, func(ref string, place treePlace) {
		chunk := chunkRefOf(place)
		spec, err := specializationTree(chain, maps, &chunk)
		if err != nil {
			return
		}
		for ss := range indexTree(spec, false) {
			for _, kind := range []string{"apply", "effective", "depend"} {
				uris = append(uris, "blocks://"+kind+"/"+tail+"/"+ref+"/"+ss)
			}
		}
	})
	leaves(itemTree, func(ref string, _ treePlace) { uris = append(uris, "blocks://refer/"+tail+"/"+ref) })
	leaves(fileTree, func(ref string, _ treePlace) { uris = append(uris, "file://"+tail+"/"+ref) })
	leaves(labelTree, func(ref string, _ treePlace) { uris = append(uris, "blocks://specialization/"+tail+"/"+ref) })
	return uris
}

// selectBlocks picks from the chain the blocks that refs ask for:
// an item, a label path, or a chunk followed by a label path.
func selectBlocks(kind string, chain []Block, maps []Mapping, refs []string) ([]Block, error) {
	var got []Block
	switch kind {
	case "refer":
		item, err := itemByRef(chain, maps, refs[0])
		if err != nil {
			return nil, err
		}
		for _, b := range chain {
			for _, it := range b.Items {
				if sameItem(it, item) {
					got = append(got, b)
					break
				}
			}
		}
		return got, nil
	case "specialization":
		labels, err := labelsByRef(chain, maps, refs[0])
		if err != nil {
			return nil, err
		}
		for _, b := range chain {
			if len(b.Ref.Labels) == len(labels) && isSuffix(b.Ref.Labels, labels) {
				got = append(got, b)
			}
		}
		return got, nil
	}
	ref, err := chunkByRef(chain, maps, refs[0])
	if err != nil {
		return nil, err
	}
	if ref.Labels, err = labelsByRef(chain, maps, refs[1]); err != nil {
		return nil, err
	}
	switch kind {
	case "apply":
		return matchingBlocks(chain, ref), nil
	case "effective":
		got, _ = effectiveBlocks(chain, ref)
		return got, nil
	}
	for _, b := range chain {
		if blockDepends(b, ref) {
			got = append(got, b)
		}
	}
	return got, nil
}

// blockList renders blocks for a resource of node. A block is found again
// among the nodes of the chain by its archive and line.
func blockList(w *World, node *Node, blocks []Block) []blockJSON {
	owner := map[string]string{}
	rev, _ := w.ancestry(formatNodePath(nodePath(*node)))
	for _, n := range rev[min(1, len(rev)):] {
		for _, b := range n.Own {
			owner[fmt.Sprintf("%s:%d", b.Source, b.StartLine)] = nodeRef(*n)
		}
	}
	out := make([]blockJSON, len(blocks))
	for i, b := range blocks {
		out[i] = blockJSON{
			Ref:     blockRef(b),
			Node:    owner[fmt.Sprintf("%s:%d", b.Source, b.StartLine)],
			Comment: strings.TrimSpace(b.Comment),
			Text:    strings.TrimRight(b.Raw, " \t\r\n"),
		}
	}
	return out
}

func inputSchema[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	return s
}

// plainArrays publishes every slice as an array.
// A slice infers "type": ["null", "array"], and some clients accept only
// one type per schema; a missing slice already stands for an empty one.
func plainArrays(s *jsonschema.Schema) *jsonschema.Schema {
	if len(s.Types) > 0 && slices.Contains(s.Types, "array") {
		s.Types, s.Type = nil, "array"
	}
	for _, prop := range s.Properties {
		plainArrays(prop)
	}
	if s.Items != nil {
		plainArrays(s.Items)
	}
	return s
}

// labelTree is the specialization tree of a node: every label path in use.
func labelTree(blocks []Block, maps []Mapping) (treeJSON, error) {
	return specializationTree(blocks, maps, nil)
}

// refOf is the ref of an object: ten hex digits of a hash over its kind and
// its names. A ref depends on nothing else, so it stays the same until the
// object is renamed.
func refOf(kind string, names ...string) string {
	h := sha256.New()
	h.Write([]byte(kind))
	for _, name := range names {
		h.Write([]byte{0})
		h.Write([]byte(name))
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

// blockRef hashes the text of a block without the blank after its end
// control, which changes when the block stops being the last of its archive.
func blockRef(b Block) string {
	return refOf("block", strings.TrimRight(b.Raw, " \t\r\n"))
}

func compositionRef(w *World) string {
	return refOf("composition", w.Dir)
}

func nodeRef(n Node) string {
	return refOf("node", nodePath(n)...)
}

// assignRefs gives every entry of a tree its ref. A branch and a leaf of the
// same path are different objects, so their kinds differ.
// includeRoot is as in indexTree.
func assignRefs(t *treeJSON, branch, leaf string, includeRoot bool) {
	var walk func(*treeJSON, []string)
	walk = func(node *treeJSON, path []string) {
		kind := branch
		if node.Leaf {
			kind = leaf
		}
		node.Ref = refOf(kind, path...)
		for i := range node.SubTrees {
			sub := &node.SubTrees[i]
			walk(sub, append(append([]string{}, path...), sub.Name))
		}
	}
	var start []string
	if includeRoot && t.Name != "" {
		start = []string{t.Name}
	}
	walk(t, start)
}

func nodeByRef(w *World, ref string) (*Node, error) {
	for i := range w.Nodes {
		if nodeRef(w.Nodes[i]) == ref {
			return &w.Nodes[i], nil
		}
	}
	return nil, fmt.Errorf("unknown node '%s'", ref)
}

func chunkByRef(blocks []Block, maps []Mapping, ref string) (ChunkRef, error) {
	tree, err := chunkTree(blocks, maps)
	if err != nil {
		return ChunkRef{}, err
	}
	place, ok := indexTree(tree, false)[ref]
	if !ok || !place.leaf {
		return ChunkRef{}, fmt.Errorf("unknown chunk '%s'", ref)
	}
	return chunkRefOf(place), nil
}

func fileByRef(blocks []Block, maps []Mapping, ref string) (string, error) {
	tree, err := fileTree(blocks, maps)
	if err != nil {
		return "", err
	}
	place, ok := indexTree(tree, false)[ref]
	if !ok || !place.leaf {
		return "", fmt.Errorf("unknown file '%s'", ref)
	}
	return strings.Join(place.names, "/"), nil
}

func itemByRef(blocks []Block, maps []Mapping, ref string) ([]Name, error) {
	tree, err := itemTree(blocks, maps)
	if err != nil {
		return nil, err
	}
	place, ok := indexTree(tree, false)[ref]
	if !ok || !place.leaf {
		return nil, fmt.Errorf("unknown item '%s'", ref)
	}
	return toNames(place.names), nil
}

func toNames(names []string) []Name {
	out := make([]Name, len(names))
	for i, name := range names {
		out[i] = Name(name)
	}
	return out
}

// labelsByRef resolves a ref of the specialization tree to labels,
// the most specific first, as a reference writes them.
func labelsByRef(blocks []Block, maps []Mapping, ref string) ([]Name, error) {
	tree, err := labelTree(blocks, maps)
	if err != nil {
		return nil, err
	}
	place, ok := indexTree(tree, false)[ref]
	if !ok {
		return nil, fmt.Errorf("unknown specialization '%s'", ref)
	}
	labels := make([]Name, len(place.names))
	for i, name := range place.names {
		labels[len(place.names)-1-i] = Name(name)
	}
	return labels, nil
}
