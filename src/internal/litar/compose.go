package litar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type composerFile struct {
	Description string         `json:"description"`
	Ground      *string        `json:"ground"`
	Nodes       []composerNode `json:"nodes"`
}

type composerNode struct {
	Name        string            `json:"name"`
	Archive     string            `json:"archive"`
	Description string            `json:"description"`
	Mappings    []composerMapping `json:"mappings"`
	Base        basePath          `json:"base"`
}

// basePath is the path from the root to the direct parent.
// An empty array is the root. Null and a string are rejected.
type basePath struct {
	names []string
	set   bool
}

type composerMapping struct {
	Path    string   `json:"path"`
	FileSet []string `json:"fileset"`
	Origin  string   `json:"origin"`
}

type groundFrame struct {
	ident string
	name  string
}

func (b *basePath) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("base must be an array")
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return fmt.Errorf("base must be an array")
	}
	if names == nil {
		names = []string{}
	}
	b.names = names
	b.set = true
	return nil
}

func (b basePath) MarshalJSON() ([]byte, error) {
	names := b.names
	if names == nil {
		names = []string{}
	}
	return json.Marshal(names)
}

// requireLocal returns a node declared in this composer file.
// A node that arrived through ground is visible, and it is not edited here.
func (w *World) requireLocal(ref string) (*Node, error) {
	i, err := w.findIndex(ref)
	if err != nil {
		return nil, err
	}
	if i < w.localAt {
		return nil, fmt.Errorf("node '%s' is defined in the ground", w.Nodes[i].Name)
	}
	return &w.Nodes[i], nil
}

// lookup finds a node by its full path, the names from the root joined by "/".
func (w *World) lookup(ref string) (*Node, error) {
	i, err := w.findIndex(ref)
	if err != nil {
		return nil, err
	}
	return &w.Nodes[i], nil
}

func (w *World) findIndex(ref string) (int, error) {
	for i := range w.Nodes {
		if formatNodePath(nodePath(w.Nodes[i])) == ref {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unknown node '%s'", ref)
}

func (w *World) root() (*Node, error) {
	i, err := w.rootIndex()
	if err != nil {
		return nil, err
	}
	return &w.Nodes[i], nil
}

func (w *World) rootIndex() (int, error) {
	found := -1
	for i := range w.Nodes {
		if len(w.Nodes[i].Base) == 0 {
			if found >= 0 {
				return 0, fmt.Errorf("composer has more than one root")
			}
			found = i
		}
	}
	if found < 0 {
		return 0, fmt.Errorf("composer has no root")
	}
	return found, nil
}

func (w *World) rootName() (string, error) {
	n, err := w.root()
	if err != nil {
		return "", err
	}
	return formatNodePath(nodePath(*n)), nil
}

// ancestry returns the node, then its base, up to the root.
func (w *World) ancestry(ref string) ([]*Node, error) {
	var n *Node
	var err error
	if ref == "" {
		n, err = w.root()
	} else {
		n, err = w.lookup(ref)
	}
	if err != nil {
		return nil, err
	}
	var rev []*Node
	seen := map[string]bool{}
	for {
		key := formatNodePath(nodePath(*n))
		if seen[key] {
			return nil, fmt.Errorf("cycle in node bases: %s", key)
		}
		seen[key] = true
		rev = append(rev, n)
		if len(n.Base) == 0 {
			return rev, nil
		}
		n, err = w.lookup(formatNodePath(n.Base))
		if err != nil {
			return nil, err
		}
	}
}

// Chain is the base chain followed by the node's own blocks.
func (w *World) Chain(name string) ([]Block, error) {
	rev, err := w.ancestry(name)
	if err != nil {
		return nil, err
	}
	var blocks []Block
	for i := len(rev) - 1; i >= 0; i-- {
		blocks = append(blocks, rev[i].Own...)
	}
	return blocks, nil
}

// Mappings is the base chain's mappings followed by the node's own.
func (w *World) Mappings(name string) ([]Mapping, error) {
	rev, err := w.ancestry(name)
	if err != nil {
		return nil, err
	}
	var maps []Mapping
	for i := len(rev) - 1; i >= 0; i-- {
		maps = append(maps, rev[i].Mappings...)
	}
	return maps, nil
}

func (w *World) save() error {
	local := w.Nodes[w.localAt:]
	raw := make([]composerNode, len(local))
	for i, n := range local {
		maps := make([]composerMapping, len(n.Mappings))
		for j, m := range n.Mappings {
			sets := m.FileSet
			if sets == nil {
				sets = []string{}
			}
			maps[j] = composerMapping{Path: m.Path, FileSet: sets, Origin: m.OriginSpec}
		}
		if maps == nil {
			maps = []composerMapping{}
		}
		raw[i] = composerNode{
			Name:        n.Name,
			Archive:     n.ArchiveSpec,
			Description: n.Description,
			Mappings:    maps,
			Base:        basePath{names: copyPath(n.Base), set: true},
		}
	}
	data, err := composerFileBytes(w.Description, w.Ground, raw)
	if err != nil {
		return err
	}
	return os.WriteFile(w.Composer, data, 0o644)
}

// CreateNode parses the archive, then appends the node and rewrites the composer.
// An empty base creates the root, and only when the composer has none.
// The archive file is created empty and must not already exist.
func (w *World) CreateNode(name, archive, description string, base []string) error {
	if name == "" {
		return fmt.Errorf("empty name")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("invalid node name '%s'", name)
	}
	base = copyPath(base)
	if len(base) == 0 {
		if _, err := w.root(); err == nil {
			return fmt.Errorf("composer already has a root")
		}
	} else if _, err := w.lookup(formatNodePath(base)); err != nil {
		return fmt.Errorf("base '%s' of node '%s' is not defined", formatNodePath(base), name)
	}
	for _, n := range w.Nodes {
		if n.Name == name && samePath(n.Base, base) {
			return fmt.Errorf("duplicate node '%s'", name)
		}
	}
	dir := filepath.Dir(w.Composer)
	resolved, err := createNewArchive(archive, dir)
	if err != nil {
		return err
	}
	created := true
	removeCreated := func() {
		if created {
			_ = os.Remove(resolved)
		}
	}
	var ident string
	ident, err = identityPath(resolved)
	if err != nil {
		removeCreated()
		return fmt.Errorf("could not read '%s'", resolved)
	}
	for _, n := range w.Nodes {
		if n.Archive == ident {
			removeCreated()
			label := formatNodePath(append(copyPath(base), name))
			return fmt.Errorf("nodes '%s' and '%s' share archive '%s'", formatNodePath(nodePath(n)), label, ident)
		}
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		removeCreated()
		return fmt.Errorf("could not read '%s'", resolved)
	}
	arch, err := ParseArchive(data, resolved)
	if err != nil {
		removeCreated()
		return err
	}
	w.Nodes = append(w.Nodes, Node{
		Name:        name,
		Archive:     ident,
		ArchiveSpec: archive,
		Description: description,
		Mappings:    []Mapping{},
		Base:        base,
		Own:         arch.Blocks,
		Trailing:    arch.Trailing,
	})
	if err := w.save(); err != nil {
		w.Nodes = w.Nodes[:len(w.Nodes)-1]
		removeCreated()
		return fmt.Errorf("could not write '%s'", w.Composer)
	}
	return nil
}

// DeleteNode removes the node and every node based on it
// from this composer file, then deletes each removed node's archive file.
// A node defined in the ground is left in place.
// A file that is already gone is not an error.
// If a file cannot be removed, the composer stays updated.
func (w *World) DeleteNode(name string) error {
	target, err := w.requireLocal(name)
	if err != nil {
		return err
	}
	targetPath := nodePath(*target)
	keep := append([]Node{}, w.Nodes[:w.localAt]...)
	var archives []string
	for _, n := range w.Nodes[w.localAt:] {
		if hasPathPrefix(nodePath(n), targetPath) {
			archives = append(archives, n.Archive)
			continue
		}
		keep = append(keep, n)
	}
	prev := w.Nodes
	w.Nodes = keep
	if err := w.save(); err != nil {
		w.Nodes = prev
		return fmt.Errorf("could not write '%s'", w.Composer)
	}
	var failed []string
	for _, path := range archives {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			failed = append(failed, path)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("could not remove '%s'", strings.Join(failed, "', '"))
	}
	return nil
}

// ApplyBlock appends text to the node's own archive, then reparses.
// A block that is no longer effective in that node is removed.
// A node defined in the ground is left unchanged.
// A failed parse leaves the file.
func (w *World) ApplyBlock(nodeName, text string) error {
	if text == "" {
		return fmt.Errorf("empty block text")
	}
	n, err := w.requireLocal(nodeName)
	if err != nil {
		return err
	}
	orig, err := os.ReadFile(n.Archive)
	if err != nil {
		return fmt.Errorf("could not read '%s'", n.Archive)
	}
	next := append([]byte{}, orig...)
	// An archive may end with the "@" of its last block; text joined to it
	// would turn that end control into ordinary text.
	if len(next) > 0 && !endsWithSpace(string(next)) {
		next = append(next, '\n')
	}
	next = append(next, text...)
	arch, err := ParseArchive(next, n.Archive)
	if err != nil {
		return err
	}
	savedOwn, savedTrail := n.Own, n.Trailing
	n.Own = arch.Blocks
	n.Trailing = arch.Trailing
	if err := w.storeCompact(n); err != nil {
		n.Own, n.Trailing = savedOwn, savedTrail
		return err
	}
	return nil
}

func (w *World) storeCompact(n *Node) error {
	data, arch, err := compactOwn(n)
	if err != nil {
		return err
	}
	if err := writeFileKeepMode(n.Archive, data); err != nil {
		return fmt.Errorf("could not write '%s'", n.Archive)
	}
	n.Own = arch.Blocks
	n.Trailing = arch.Trailing
	return nil
}

func (w *World) rewriteOwn(n *Node, keep func(Block, int) bool) error {
	var kept []Block
	for i, bl := range n.Own {
		if keep(bl, i) {
			kept = append(kept, bl)
		}
	}
	savedOwn, savedTrail := n.Own, n.Trailing
	n.Own = kept
	if err := w.storeCompact(n); err != nil {
		n.Own, n.Trailing = savedOwn, savedTrail
		return err
	}
	return nil
}

// DeleteChunk removes own blocks that match ref, the same set -l prints.
func (w *World) DeleteChunk(nodeName string, ref ChunkRef) error {
	n, err := w.requireLocal(nodeName)
	if err != nil {
		return err
	}
	return w.rewriteOwn(n, func(b Block, _ int) bool {
		return !matchRef(b.Ref, ref)
	})
}

// DeleteChunkAll removes every own block of ref's module and chunk,
// whatever labels it carries.
func (w *World) DeleteChunkAll(nodeName string, ref ChunkRef) error {
	n, err := w.requireLocal(nodeName)
	if err != nil {
		return err
	}
	return w.rewriteOwn(n, func(b Block, _ int) bool {
		return b.Ref.Chunk != ref.Chunk || !modulesEqual(b.Ref.Module, ref.Module)
	})
}

// DeleteFile removes file blocks with that path and blocks applied to
// the root chunk of that path.
func (w *World) DeleteFile(nodeName, path string) error {
	n, err := w.requireLocal(nodeName)
	if err != nil {
		return err
	}
	ref := ChunkRef{Chunk: Name(path)}
	return w.rewriteOwn(n, func(b Block, _ int) bool {
		if b.IsFile && string(b.Path) == path {
			return false
		}
		return !matchRef(b.Ref, ref)
	})
}

// DumpArchives rewrites each node's own archive, dropping a block that
// is not an effective block of its own chunk reference in that node
// and ordering the rest as orderOwn does.
func (w *World) DumpArchives() error {
	type pending struct {
		n    *Node
		data []byte
		arch *Archive
	}
	var all []pending
	for i := w.localAt; i < len(w.Nodes); i++ {
		n := &w.Nodes[i]
		data, arch, err := compactOwn(n)
		if err != nil {
			return err
		}
		all = append(all, pending{n: n, data: data, arch: arch})
	}
	for _, p := range all {
		if err := writeFileKeepMode(p.n.Archive, p.data); err != nil {
			return fmt.Errorf("could not write '%s'", p.n.Archive)
		}
		p.n.Own = p.arch.Blocks
		p.n.Trailing = p.arch.Trailing
	}
	return nil
}

// Describe replaces the description of a node, or of the composition when nodeName is empty.
func (w *World) Describe(nodeName, description string) error {
	if nodeName == "" {
		w.Description = description
		return w.save()
	}
	n, err := w.requireLocal(nodeName)
	if err != nil {
		return err
	}
	n.Description = description
	return w.save()
}

// LoadComposer reads a composer file.
// The combined ground and local nodes have exactly one root.
func LoadComposer(path string) (*World, error) {
	return loadComposer(path, false)
}

// LoadComposerAllowEmpty accepts a composer with a null ground and no nodes.
func LoadComposerAllowEmpty(path string) (*World, error) {
	return loadComposer(path, true)
}

func loadComposer(path string, allowEmpty bool) (*World, error) {
	ident, err := identityPath(path)
	if err != nil {
		return nil, fmt.Errorf("could not read composer '%s': %v", path, err)
	}
	return loadAt(path, ident, allowEmpty, nil)
}

func loadAt(path, ident string, allowEmpty bool, stack []groundFrame) (*World, error) {
	for i, frame := range stack {
		if frame.ident != ident {
			continue
		}
		var parts []string
		for _, g := range stack[i:] {
			parts = append(parts, g.name)
		}
		parts = append(parts, frame.name)
		return nil, fmt.Errorf("cycle in grounds: %s", strings.Join(parts, " -> "))
	}
	dirIdent, err := identityPath(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("could not read composer '%s': %v", path, err)
	}
	var file composerFile
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read composer '%s': %v", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("could not read composer '%s': %v", path, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("could not read composer '%s': trailing data", path)
	}
	var ground *string
	var groundNodes []Node
	if file.Ground != nil {
		gname := *file.Ground
		ground = &gname
		if err := validGroundName(gname); err != nil {
			return nil, err
		}
		gpath, err := findGround(gname)
		if err != nil {
			return nil, err
		}
		gident, err := identityPath(gpath)
		if err != nil {
			return nil, fmt.Errorf("could not read composer '%s': %v", gpath, err)
		}
		gw, err := loadAt(gpath, gident, false, append(stack, groundFrame{ident: ident, name: gname}))
		if err != nil {
			return nil, err
		}
		groundNodes = gw.Nodes
	}
	if ground == nil && len(file.Nodes) == 0 {
		if allowEmpty {
			return &World{Composer: path, Dir: dirIdent, Description: file.Description}, nil
		}
		return nil, fmt.Errorf("composer has no root")
	}
	if err := prepareNodes(file.Nodes); err != nil {
		return nil, err
	}
	// Names and bases are checked before archives are read.
	if err := validateGraph(graphNodes(groundNodes, file.Nodes)); err != nil {
		return nil, err
	}
	local, err := nodesFrom(path, file.Nodes)
	if err != nil {
		return nil, err
	}
	all := make([]Node, 0, len(groundNodes)+len(local))
	all = append(all, groundNodes...)
	all = append(all, local...)
	if err := distinctArchives(all); err != nil {
		return nil, err
	}
	return &World{
		Composer:    path,
		Dir:         dirIdent,
		Description: file.Description,
		Ground:      ground,
		Nodes:       all,
		localAt:     len(groundNodes),
	}, nil
}

func validGroundName(name string) error {
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return fmt.Errorf("invalid ground '%s'", name)
	}
	return nil
}

func findGround(name string) (string, error) {
	for _, dir := range librarySearchDirs() {
		path := filepath.Join(dir, name, "composer.json")
		if isRegular(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("could not find ground '%s'", name)
}

func prepareNodes(raw []composerNode) error {
	for _, n := range raw {
		if n.Name == "" {
			return fmt.Errorf("empty name")
		}
		if !n.Base.set {
			return fmt.Errorf("node '%s' is missing base", n.Name)
		}
	}
	return nil
}

func graphNodes(ground []Node, raw []composerNode) []Node {
	all := make([]Node, 0, len(ground)+len(raw))
	for _, n := range ground {
		all = append(all, Node{Name: n.Name, Base: copyPath(n.Base)})
	}
	for _, n := range raw {
		all = append(all, Node{Name: n.Name, Base: copyPath(n.Base.names)})
	}
	return all
}

func validateGraph(nodes []Node) error {
	seen := map[string]int{}
	for i, n := range nodes {
		if n.Name == "" {
			return fmt.Errorf("empty name")
		}
		if strings.Contains(n.Name, "/") {
			return fmt.Errorf("invalid node name '%s'", n.Name)
		}
		key := formatNodePath(nodePath(n))
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate node '%s'", n.Name)
		}
		seen[key] = i
	}
	var roots int
	for _, n := range nodes {
		if len(n.Base) == 0 {
			roots++
			continue
		}
		if _, ok := seen[formatNodePath(n.Base)]; !ok {
			return fmt.Errorf("base '%s' of node '%s' is not defined", formatNodePath(n.Base), n.Name)
		}
	}
	if roots == 0 {
		return fmt.Errorf("composer has no root")
	}
	if roots > 1 {
		return fmt.Errorf("composer has more than one root")
	}
	return nil
}

func nodesFrom(composerPath string, raw []composerNode) ([]Node, error) {
	dir := filepath.Dir(composerPath)
	nodes := make([]Node, len(raw))
	for i, n := range raw {
		base := copyPath(n.Base.names)
		var (
			ident string
			arch  *Archive
		)
		resolved, err := resolveArchive(n.Archive, dir)
		if err != nil {
			return nil, err
		}
		ident, err = identityPath(resolved)
		if err != nil {
			return nil, fmt.Errorf("could not read '%s'", resolved)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("could not read '%s'", resolved)
		}
		arch, err = ParseArchive(data, resolved)
		if err != nil {
			return nil, err
		}
		maps := make([]Mapping, len(n.Mappings))
		for j, m := range n.Mappings {
			origin := m.Origin
			if origin != "" && !filepath.IsAbs(origin) {
				origin = filepath.Join(dir, origin)
			}
			sets := m.FileSet
			if sets == nil {
				sets = []string{}
			}
			maps[j] = Mapping{
				Path:       m.Path,
				FileSet:    sets,
				Origin:     origin,
				OriginSpec: m.Origin,
			}
		}
		nodes[i] = Node{
			Name:        n.Name,
			Archive:     ident,
			ArchiveSpec: n.Archive,
			Description: n.Description,
			Mappings:    maps,
			Base:        base,
			Own:         arch.Blocks,
			Trailing:    arch.Trailing,
		}
	}
	return nodes, nil
}

func distinctArchives(nodes []Node) error {
	archives := map[string]string{}
	for _, n := range nodes {
		label := formatNodePath(nodePath(n))
		if other, ok := archives[n.Archive]; ok {
			return fmt.Errorf("nodes '%s' and '%s' share archive '%s'", other, label, n.Archive)
		}
		archives[n.Archive] = label
	}
	return nil
}

func resolveArchive(spec, composerDir string) (string, error) {
	if spec == "" || filepath.IsAbs(spec) {
		return "", fmt.Errorf("could not find archive '%s'", spec)
	}
	path := filepath.Join(composerDir, spec)
	if isRegular(path) {
		return path, nil
	}
	return "", fmt.Errorf("could not find archive '%s'", spec)
}

// createNewArchive writes an empty archive beside the composer file.
// The file must not already exist.
func createNewArchive(spec, composerDir string) (string, error) {
	if spec == "" || filepath.IsAbs(spec) {
		return "", fmt.Errorf("could not find archive '%s'", spec)
	}
	path := filepath.Join(composerDir, spec)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("archive '%s' already exists", spec)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("could not read '%s'", path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("could not write '%s'", path)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("could not write '%s'", path)
	}
	return path, nil
}

func librarySearchDirs() []string {
	dirs := []string{"."}
	if inc := os.Getenv("LITAR_LIBRARY"); inc != "" {
		for _, dir := range filepath.SplitList(inc) {
			if dir == "" {
				dir = "."
			}
			dirs = append(dirs, dir)
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".litar"),
			filepath.Join(home, ".local", "share", "litar"),
		)
	}
	dirs = append(dirs, "/usr/local/share/litar", "/usr/share/litar")
	return dirs
}

func isRegular(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

func identityPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved), nil
	}
	return abs, nil
}

// FindArchive loads the composition that declares archive and returns the
// path of the declaring node. It looks for composer.json from the archive's
// directory upward; the nearest composer that declares the archive as one of
// its own nodes wins.
func FindArchive(archive string) (*World, string, error) {
	ident, err := identityPath(archive)
	if err != nil || !isRegular(ident) {
		return nil, "", fmt.Errorf("could not find archive '%s'", archive)
	}
	var loadErr error
	dir := filepath.Dir(ident)
	for {
		comp := filepath.Join(dir, "composer.json")
		if isRegular(comp) {
			w, err := LoadComposer(comp)
			if err != nil {
				if loadErr == nil {
					loadErr = err
				}
			} else {
				for i := w.localAt; i < len(w.Nodes); i++ {
					if w.Nodes[i].Archive == ident {
						return w, formatNodePath(nodePath(w.Nodes[i])), nil
					}
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if loadErr != nil {
		return nil, "", loadErr
	}
	return nil, "", fmt.Errorf("no composition declares archive '%s'", archive)
}

func composerFileBytes(description string, ground *string, nodes []composerNode) ([]byte, error) {
	if nodes == nil {
		nodes = []composerNode{}
	}
	data, err := json.MarshalIndent(composerFile{
		Description: description,
		Ground:      ground,
		Nodes:       nodes,
	}, "", "    ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func emptyComposerBytes() []byte {
	data, err := composerFileBytes("", nil, nil)
	if err != nil {
		panic(err)
	}
	return data
}

// SpecifyComposer loads an existing composer file.
// A directory uses the composer.json inside it.
func SpecifyComposer(path string) (*World, error) {
	if path == "" {
		return nil, fmt.Errorf("missing path")
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("could not read composer '%s': %v", path, err)
	}
	if st.IsDir() {
		path = filepath.Join(path, "composer.json")
		st, err = os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("could not read composer '%s': %v", path, err)
		}
	}
	if st.IsDir() {
		return nil, fmt.Errorf("could not read composer '%s': is a directory", path)
	}
	return LoadComposerAllowEmpty(path)
}

// CreateComposedArchive creates dir/composer.json when that file is absent,
// then loads it. An empty node list is allowed.
// An empty ground is stored as null.
// A named ground is resolved on the library search path before the file is written.
// If the new file cannot be loaded, it is removed.
func CreateComposedArchive(dir, ground string) (*World, error) {
	if dir == "" {
		return nil, fmt.Errorf("missing path")
	}
	var stored *string
	if ground != "" {
		if err := validGroundName(ground); err != nil {
			return nil, err
		}
		if _, err := findGround(ground); err != nil {
			return nil, err
		}
		name := ground
		stored = &name
	}
	createdDir := false
	comp := filepath.Join(dir, "composer.json")
	st, err := os.Stat(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("could not write '%s': %v", dir, err)
		}
		createdDir = true
	} else if !st.IsDir() {
		return nil, fmt.Errorf("could not write '%s': not a directory", dir)
	}
	if _, err := os.Stat(comp); err == nil {
		return nil, fmt.Errorf("composition '%s' already exists", dir)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("could not write '%s': %v", comp, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("could not write '%s': %v", dir, err)
	}
	removeNew := func() {
		_ = os.Remove(comp)
		if createdDir {
			_ = os.Remove(dir)
		}
	}
	data, err := composerFileBytes("", stored, nil)
	if err != nil {
		removeNew()
		return nil, err
	}
	if err := os.WriteFile(comp, data, 0o644); err != nil {
		removeNew()
		return nil, fmt.Errorf("could not write '%s': %v", comp, err)
	}
	w, err := LoadComposerAllowEmpty(comp)
	if err != nil {
		removeNew()
		return nil, err
	}
	return w, nil
}

func writeFileKeepMode(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	return os.WriteFile(path, data, mode)
}

// compactOwn drops blocks that are not effective for their own
// chunk reference among this node's own blocks, then orders the rest.
func compactOwn(n *Node) ([]byte, *Archive, error) {
	var kept []Block
	own := orderOwn(n.Own)
	cache := map[string]map[int]bool{}
	for bi, bl := range own {
		k := bl.Ref.key()
		set, ok := cache[k]
		if !ok {
			set = effectiveIndexSet(own, bl.Ref)
			cache[k] = set
		}
		if set[bi] {
			kept = append(kept, bl)
		}
	}
	kept = orderOwn(kept)
	var b strings.Builder
	for i, bl := range kept {
		b.WriteString(bl.Comment)
		b.WriteString(bl.Raw)
		// A block that closed at the end of the archive has no
		// whitespace after its "@"; the next text would join it.
		last := i == len(kept)-1 && n.Trailing == ""
		if !last && !endsWithSpace(bl.Raw) {
			b.WriteByte('\n')
		}
	}
	b.WriteString(n.Trailing)
	data := []byte(b.String())
	arch, err := ParseArchive(data, n.Archive)
	if err != nil {
		return nil, nil, err
	}
	if arch.Text() != string(data) {
		return nil, nil, fmt.Errorf("could not write '%s'", n.Archive)
	}
	return data, arch, nil
}

func endsWithSpace(s string) bool {
	if s == "" {
		return false
	}
	switch s[len(s)-1] {
	case ' ', '\t', '\r', '\n':
		return true
	}
	return false
}

// chunkKey identifies a module and chunk, whatever the labels.
func chunkKey(ref ChunkRef) string {
	return ChunkRef{Module: ref.Module, Chunk: ref.Chunk}.key()
}

// orderOwn places a block after the blocks whose content includes its chunk.
// Blocks of one module and chunk follow specializationOrder, because that order
// decides the content and the effective blocks; when inclusion is circular,
// the rule gives way to that order. Otherwise blocks keep their order.
func orderOwn(blocks []Block) []Block {
	n := len(blocks)
	keys := make([]string, n)
	byChunk := map[string][]int{}
	for i, b := range blocks {
		keys[i] = chunkKey(b.Ref)
		byChunk[keys[i]] = append(byChunk[keys[i]], i)
	}
	// chainPrev[i] is the earlier block of the same chunk, or -1.
	chainPrev := make([]int, n)
	for _, idx := range byChunk {
		order := specializationOrder(blocks, idx)
		chainPrev[order[0]] = -1
		for j := 1; j < len(order); j++ {
			chainPrev[order[j]] = order[j-1]
		}
	}
	includers := make([]map[int]bool, n)
	for a, b := range blocks {
		for _, part := range b.Parts {
			if part.Ref == nil || part.Ref.Chunk == "" {
				continue
			}
			inner := resolveBody(part.Ref.ChunkRef, nil, b.Ref.Module)
			k := chunkKey(inner)
			if k == keys[a] {
				continue
			}
			for _, c := range byChunk[k] {
				if includers[c] == nil {
					includers[c] = map[int]bool{}
				}
				includers[c][a] = true
			}
		}
	}
	placed := make([]bool, n)
	ready := func(i int, strict bool) bool {
		if placed[i] || (chainPrev[i] >= 0 && !placed[chainPrev[i]]) {
			return false
		}
		if !strict {
			return true
		}
		for a := range includers[i] {
			if !placed[a] {
				return false
			}
		}
		return true
	}
	out := make([]Block, 0, n)
	for len(out) < n {
		next := -1
		for i := 0; i < n && next < 0; i++ {
			if ready(i, true) {
				next = i
			}
		}
		for i := 0; i < n && next < 0; i++ {
			if ready(i, false) {
				next = i
			}
		}
		placed[next] = true
		out = append(out, blocks[next])
	}
	return out
}

// effectiveIndexSet lists blocks that still contribute to query.
func effectiveIndexSet(blocks []Block, query ChunkRef) map[int]bool {
	var idx []int
	for i, b := range blocks {
		if !matchRef(b.Ref, query) {
			continue
		}
		if b.Op == OpSet {
			idx = []int{i}
		} else {
			idx = append(idx, i)
		}
	}
	set := make(map[int]bool, len(idx))
	for _, i := range idx {
		set[i] = true
	}
	return set
}

// specializationOrder lists idx, the blocks of one chunk, so that a block follows
// the blocks whose labels are a proper suffix of its labels; otherwise idx keeps its order.
func specializationOrder(blocks []Block, idx []int) []int {
	k := len(idx)
	before := make([]int, k)
	after := make([][]int, k)
	for p := 0; p < k; p++ {
		for q := 0; q < k; q++ {
			lp, lq := blocks[idx[p]].Ref.Labels, blocks[idx[q]].Ref.Labels
			if len(lp) < len(lq) && isSuffix(lp, lq) {
				after[p] = append(after[p], q)
				before[q]++
			}
		}
	}
	order := make([]int, 0, k)
	placed := make([]bool, k)
	for len(order) < k {
		for q := 0; q < k; q++ {
			if !placed[q] && before[q] == 0 {
				placed[q] = true
				order = append(order, idx[q])
				for _, r := range after[q] {
					before[r]--
				}
				break
			}
		}
	}
	return order
}
