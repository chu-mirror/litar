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
	Ground *string        `json:"ground"`
	Nodes  []composerNode `json:"nodes"`
}

type composerNode struct {
	Name        string            `json:"name"`
	Archive     string            `json:"archive"`
	Description string            `json:"description"`
	Mappings    []composerMapping `json:"mappings"`
	Base        *string           `json:"base"`
}

type composerMapping struct {
	Path    string   `json:"path"`
	FileSet []string `json:"fileset"`
	Origin  string   `json:"origin"`
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

type groundFrame struct {
	ident string
	name  string
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read composer '%s': %v", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var file composerFile
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
			return &World{Composer: path}, nil
		}
		return nil, fmt.Errorf("composer has no root")
	}
	// Names, bases, and cycles are checked before archives are read,
	// so a cycle is reported even when the nodes share one file.
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
		Composer: path,
		Ground:   ground,
		Nodes:    all,
		localAt:  len(groundNodes),
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

func graphNodes(ground []Node, raw []composerNode) []Node {
	all := make([]Node, 0, len(ground)+len(raw))
	for _, n := range ground {
		all = append(all, Node{Name: n.Name, Base: n.Base})
	}
	for _, n := range raw {
		base := ""
		if n.Base != nil {
			base = *n.Base
		}
		all = append(all, Node{Name: n.Name, Base: base})
	}
	return all
}

func validateGraph(nodes []Node) error {
	seen := map[string]int{}
	for i, n := range nodes {
		if n.Name == "" {
			return fmt.Errorf("empty name")
		}
		if _, ok := seen[n.Name]; ok {
			return fmt.Errorf("duplicate node '%s'", n.Name)
		}
		seen[n.Name] = i
	}
	var roots []string
	for _, n := range nodes {
		if n.Base == "" {
			roots = append(roots, n.Name)
			continue
		}
		if _, ok := seen[n.Base]; !ok {
			return fmt.Errorf("base '%s' of node '%s' is not defined", n.Base, n.Name)
		}
	}
	if len(roots) == 0 {
		return fmt.Errorf("composer has no root")
	}
	if len(roots) > 1 {
		return fmt.Errorf("composer has more than one root")
	}
	if err := nodeCycles(nodes); err != nil {
		return err
	}
	return nodeReachable(nodes, roots[0])
}

func nodesFrom(composerPath string, raw []composerNode) ([]Node, error) {
	dir := filepath.Dir(composerPath)
	nodes := make([]Node, len(raw))
	for i, n := range raw {
		base := ""
		if n.Base != nil {
			base = *n.Base
		}
		resolved, err := resolveArchive(n.Archive, dir)
		if err != nil {
			return nil, err
		}
		ident, err := identityPath(resolved)
		if err != nil {
			return nil, fmt.Errorf("could not read '%s'", resolved)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("could not read '%s'", resolved)
		}
		arch, err := ParseArchive(data, resolved)
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
		if other, ok := archives[n.Archive]; ok {
			return fmt.Errorf("nodes '%s' and '%s' share archive '%s'", other, n.Name, n.Archive)
		}
		archives[n.Archive] = n.Name
	}
	return nil
}

func nodeCycles(nodes []Node) error {
	index := map[string]int{}
	for i, n := range nodes {
		index[n.Name] = i
	}
	const white, gray, black = 0, 1, 2
	color := make([]int, len(nodes))
	var stack []string
	var dfs func(i int) error
	dfs = func(i int) error {
		color[i] = gray
		stack = append(stack, nodes[i].Name)
		base := nodes[i].Base
		if base != "" {
			j := index[base]
			switch color[j] {
			case gray:
				k := 0
				for k < len(stack) && stack[k] != base {
					k++
				}
				cyc := append(append([]string{}, stack[k:]...), base)
				return fmt.Errorf("cycle in node bases: %s", strings.Join(cyc, " -> "))
			case white:
				if err := dfs(j); err != nil {
					return err
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[i] = black
		return nil
	}
	for i := range nodes {
		if color[i] == white {
			if err := dfs(i); err != nil {
				return err
			}
		}
	}
	return nil
}

func nodeReachable(nodes []Node, root string) error {
	children := map[string][]string{}
	for _, n := range nodes {
		if n.Base != "" {
			children[n.Base] = append(children[n.Base], n.Name)
		}
	}
	seen := map[string]bool{root: true}
	queue := []string{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, child := range children[cur] {
			if !seen[child] {
				seen[child] = true
				queue = append(queue, child)
			}
		}
	}
	for _, n := range nodes {
		if !seen[n.Name] {
			return fmt.Errorf("node '%s' is not reachable from the root", n.Name)
		}
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

// createMissingArchive writes an empty archive beside the composer file.
// The archive name is relative to that file.
func createMissingArchive(spec, composerDir string) (string, error) {
	if spec == "" || filepath.IsAbs(spec) {
		return "", fmt.Errorf("could not find archive '%s'", spec)
	}
	path := filepath.Join(composerDir, spec)
	st, err := os.Stat(path)
	if err == nil {
		if st.Mode().IsRegular() {
			return path, nil
		}
		return "", fmt.Errorf("could not find archive '%s'", spec)
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("could not read '%s'", path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) && isRegular(path) {
			return path, nil
		}
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

// requireLocal returns a node declared in this composer file.
// A node that arrived through ground is visible, and it is not edited here.
func (w *World) requireLocal(name string) (*Node, error) {
	for i := range w.Nodes {
		if w.Nodes[i].Name != name {
			continue
		}
		if i < w.localAt {
			return nil, fmt.Errorf("node '%s' is defined in the ground", name)
		}
		return &w.Nodes[i], nil
	}
	return nil, fmt.Errorf("unknown node '%s'", name)
}

func (w *World) lookup(name string) (*Node, error) {
	for i := range w.Nodes {
		if w.Nodes[i].Name == name {
			return &w.Nodes[i], nil
		}
	}
	return nil, fmt.Errorf("unknown node '%s'", name)
}

func (w *World) rootName() (string, error) {
	for _, n := range w.Nodes {
		if n.Base == "" {
			return n.Name, nil
		}
	}
	return "", fmt.Errorf("composer has no root")
}

// ancestry returns the node, then its base, up to the root.
func (w *World) ancestry(name string) ([]*Node, error) {
	if name == "" {
		var err error
		name, err = w.rootName()
		if err != nil {
			return nil, err
		}
	}
	var rev []*Node
	seen := map[string]bool{}
	for name != "" {
		if seen[name] {
			return nil, fmt.Errorf("cycle in node bases: %s", name)
		}
		seen[name] = true
		n, err := w.lookup(name)
		if err != nil {
			return nil, err
		}
		rev = append(rev, n)
		name = n.Base
	}
	return rev, nil
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

func composerFileBytes(ground *string, nodes []composerNode) ([]byte, error) {
	if nodes == nil {
		nodes = []composerNode{}
	}
	data, err := json.MarshalIndent(composerFile{Ground: ground, Nodes: nodes}, "", "    ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func emptyComposerBytes() []byte {
	data, err := composerFileBytes(nil, nil)
	if err != nil {
		panic(err)
	}
	return data
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
		var base *string
		if n.Base != "" {
			b := n.Base
			base = &b
		}
		raw[i] = composerNode{
			Name:        n.Name,
			Archive:     n.ArchiveSpec,
			Description: n.Description,
			Mappings:    maps,
			Base:        base,
		}
	}
	data, err := composerFileBytes(w.Ground, raw)
	if err != nil {
		return err
	}
	return os.WriteFile(w.Composer, data, 0o644)
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
	st, err := os.Stat(dir)
	createdDir := false
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("could not write '%s': %v", dir, err)
		}
		createdDir = true
	} else if !st.IsDir() {
		return nil, fmt.Errorf("could not write '%s': not a directory", dir)
	}
	comp := filepath.Join(dir, "composer.json")
	if _, err := os.Stat(comp); err == nil {
		return nil, fmt.Errorf("composed archive '%s' already exists", dir)
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
	data, err := composerFileBytes(stored, nil)
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

// CreateNode parses the archive, then appends the node and rewrites the composer.
// An empty base creates the root, and only when the composer has none.
// A missing archive file is created empty.
func (w *World) CreateNode(name, archive, description, base string) error {
	if name == "" {
		return fmt.Errorf("empty name")
	}
	if _, err := w.lookup(name); err == nil {
		return fmt.Errorf("duplicate node '%s'", name)
	}
	if base == "" {
		if _, err := w.rootName(); err == nil {
			return fmt.Errorf("composer already has a root")
		}
	} else if _, err := w.lookup(base); err != nil {
		return fmt.Errorf("base '%s' of node '%s' is not defined", base, name)
	}
	dir := filepath.Dir(w.Composer)
	resolved, err := resolveArchive(archive, dir)
	created := false
	if err != nil {
		resolved, err = createMissingArchive(archive, dir)
		if err != nil {
			return err
		}
		created = true
	}
	removeCreated := func() {
		if created {
			_ = os.Remove(resolved)
		}
	}
	ident, err := identityPath(resolved)
	if err != nil {
		removeCreated()
		return fmt.Errorf("could not read '%s'", resolved)
	}
	for _, n := range w.Nodes {
		if n.Archive == ident {
			removeCreated()
			return fmt.Errorf("nodes '%s' and '%s' share archive '%s'", n.Name, name, ident)
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
	if _, err := w.requireLocal(name); err != nil {
		return err
	}
	drop := map[string]bool{name: true}
	changed := true
	for changed {
		changed = false
		for _, n := range w.Nodes[w.localAt:] {
			if !drop[n.Name] && n.Base != "" && drop[n.Base] {
				drop[n.Name] = true
				changed = true
			}
		}
	}
	keep := append([]Node{}, w.Nodes[:w.localAt]...)
	var archives []string
	for _, n := range w.Nodes[w.localAt:] {
		if drop[n.Name] {
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

func writeFileKeepMode(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	return os.WriteFile(path, data, mode)
}

// ApplyBlock appends text to the node's own archive, then reparses.
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
	next := append(append([]byte{}, orig...), text...)
	arch, err := ParseArchive(next, n.Archive)
	if err != nil {
		return err
	}
	if err := writeFileKeepMode(n.Archive, next); err != nil {
		return fmt.Errorf("could not write '%s'", n.Archive)
	}
	n.Own = arch.Blocks
	n.Trailing = arch.Trailing
	return nil
}

func (w *World) rewriteOwn(n *Node, keep func(Block, int) bool) error {
	var b strings.Builder
	for i, bl := range n.Own {
		if keep(bl, i) {
			b.WriteString(bl.Comment)
			b.WriteString(bl.Raw)
		}
	}
	b.WriteString(n.Trailing)
	next := []byte(b.String())
	arch, err := ParseArchive(next, n.Archive)
	if err != nil {
		return err
	}
	orig, err := os.ReadFile(n.Archive)
	if err != nil {
		return fmt.Errorf("could not read '%s'", n.Archive)
	}
	if err := writeFileKeepMode(n.Archive, next); err != nil {
		return fmt.Errorf("could not write '%s'", n.Archive)
	}
	n.Own = arch.Blocks
	n.Trailing = arch.Trailing
	// Parse succeeded before the write. If a later caller needs the
	// original bytes, they are still in orig; restore on a mismatch
	// between the rendered text and what we stored.
	if arch.Text() != string(next) {
		_ = writeFileKeepMode(n.Archive, orig)
		return fmt.Errorf("could not write '%s'", n.Archive)
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

// DumpArchives rewrites each node's own archive, dropping a block that
// is not an effective block of its own chunk reference in that node.
func (w *World) DumpArchives() error {
	type pending struct {
		n    *Node
		data []byte
		arch *Archive
	}
	var all []pending
	for i := w.localAt; i < len(w.Nodes); i++ {
		n := &w.Nodes[i]
		cache := map[string]map[int]bool{}
		var b strings.Builder
		for bi, bl := range n.Own {
			k := bl.Ref.key()
			set, ok := cache[k]
			if !ok {
				set = effectiveIndexSet(n.Own, bl.Ref)
				cache[k] = set
			}
			if set[bi] {
				b.WriteString(bl.Comment)
				b.WriteString(bl.Raw)
			}
		}
		b.WriteString(n.Trailing)
		data := []byte(b.String())
		arch, err := ParseArchive(data, n.Archive)
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
