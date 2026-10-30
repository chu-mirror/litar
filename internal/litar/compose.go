package litar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

// LoadComposer reads a composer file and requires exactly one root.
func LoadComposer(path string) (*World, error) {
	return loadComposer(path, false)
}

// LoadComposerAllowEmpty accepts a composer that is an empty array.
func LoadComposerAllowEmpty(path string) (*World, error) {
	return loadComposer(path, true)
}

func loadComposer(path string, allowEmpty bool) (*World, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read composer '%s': %v", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var raw []composerNode
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("could not read composer '%s': %v", path, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("could not read composer '%s': trailing data", path)
	}
	if len(raw) == 0 {
		if allowEmpty {
			return &World{Composer: path}, nil
		}
		return nil, fmt.Errorf("composer has no root")
	}
	w, err := worldFrom(path, raw)
	if err != nil {
		return nil, err
	}
	return w, nil
}

func worldFrom(composerPath string, raw []composerNode) (*World, error) {
	seen := map[string]int{}
	for i, n := range raw {
		if n.Name == "" {
			return nil, fmt.Errorf("empty name")
		}
		if _, ok := seen[n.Name]; ok {
			return nil, fmt.Errorf("duplicate node '%s'", n.Name)
		}
		seen[n.Name] = i
	}
	var roots []string
	for _, n := range raw {
		base := ""
		if n.Base != nil {
			base = *n.Base
		}
		if base == "" {
			roots = append(roots, n.Name)
			continue
		}
		if _, ok := seen[base]; !ok {
			return nil, fmt.Errorf("base '%s' of node '%s' is not defined", base, n.Name)
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("composer has no root")
	}
	if len(roots) > 1 {
		return nil, fmt.Errorf("composer has more than one root")
	}
	if err := nodeCycles(raw); err != nil {
		return nil, err
	}
	if err := nodeReachable(raw, roots[0]); err != nil {
		return nil, err
	}

	dir := filepath.Dir(composerPath)
	nodes := make([]Node, len(raw))
	archives := map[string]string{}
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
		if other, ok := archives[ident]; ok {
			return nil, fmt.Errorf("nodes '%s' and '%s' share archive '%s'", other, n.Name, ident)
		}
		archives[ident] = n.Name
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
	return &World{Composer: composerPath, Nodes: nodes}, nil
}

func nodeCycles(raw []composerNode) error {
	index := map[string]int{}
	for i, n := range raw {
		index[n.Name] = i
	}
	const white, gray, black = 0, 1, 2
	color := make([]int, len(raw))
	var stack []string
	var dfs func(i int) error
	dfs = func(i int) error {
		color[i] = gray
		stack = append(stack, raw[i].Name)
		base := ""
		if raw[i].Base != nil {
			base = *raw[i].Base
		}
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
	for i := range raw {
		if color[i] == white {
			if err := dfs(i); err != nil {
				return err
			}
		}
	}
	return nil
}

func nodeReachable(raw []composerNode, root string) error {
	children := map[string][]string{}
	for _, n := range raw {
		base := ""
		if n.Base != nil {
			base = *n.Base
		}
		if base != "" {
			children[base] = append(children[base], n.Name)
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
	for _, n := range raw {
		if !seen[n.Name] {
			return fmt.Errorf("node '%s' is not reachable from the root", n.Name)
		}
	}
	return nil
}

func resolveArchive(spec, composerDir string) (string, error) {
	if spec == "" {
		return "", fmt.Errorf("could not find archive '%s'", spec)
	}
	if strings.HasPrefix(spec, ".") {
		path := filepath.Join(composerDir, spec)
		if isRegular(path) {
			return path, nil
		}
		return "", fmt.Errorf("could not find archive '%s'", spec)
	}
	dirs := archiveSearchDirs()
	for _, dir := range dirs {
		path := filepath.Join(dir, spec)
		if isRegular(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("could not find archive '%s'", spec)
}

// createMissingArchive writes an empty archive when resolveArchive finds none.
// A spec that starts with '.' is created beside the composer file.
// Any other spec is created in the current directory, the first search directory.
func createMissingArchive(spec, composerDir string) (string, error) {
	if spec == "" {
		return "", fmt.Errorf("could not find archive '%s'", spec)
	}
	path := filepath.Join(".", spec)
	if strings.HasPrefix(spec, ".") {
		path = filepath.Join(composerDir, spec)
	}
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

func archiveSearchDirs() []string {
	dirs := []string{"."}
	if inc := os.Getenv("LITAR_INCLUDE"); inc != "" {
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

func (w *World) save() error {
	raw := make([]composerNode, len(w.Nodes))
	for i, n := range w.Nodes {
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
	data, err := json.MarshalIndent(raw, "", "    ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(w.Composer, data, 0o644)
}

// SpecifyComposer creates an empty composer when path does not exist,
// then loads it. An empty array is allowed.
func SpecifyComposer(path string) (*World, error) {
	st, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("could not read composer '%s': %v", path, err)
		}
		if err := os.WriteFile(path, []byte("[]\n"), 0o644); err != nil {
			return nil, fmt.Errorf("could not read composer '%s': %v", path, err)
		}
	} else if st.IsDir() {
		return nil, fmt.Errorf("could not read composer '%s': is a directory", path)
	}
	return LoadComposerAllowEmpty(path)
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

// DeleteNode removes the node and every node based on it,
// then deletes each removed node's archive file.
// A file that is already gone is not an error.
// If a file cannot be removed, the composer stays updated.
func (w *World) DeleteNode(name string) error {
	if _, err := w.lookup(name); err != nil {
		return err
	}
	drop := map[string]bool{name: true}
	changed := true
	for changed {
		changed = false
		for _, n := range w.Nodes {
			if !drop[n.Name] && n.Base != "" && drop[n.Base] {
				drop[n.Name] = true
				changed = true
			}
		}
	}
	var keep []Node
	var archives []string
	for _, n := range w.Nodes {
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

func writableArchive(n *Node) error {
	if !strings.HasPrefix(n.ArchiveSpec, ".") {
		return fmt.Errorf("cannot apply a block to a node whose archive path does not start with '.' or '..'")
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

// ApplyBlock appends text to the node's archive when the archive spec
// starts with "." or "..", then reparses. A failed parse leaves the file.
func (w *World) ApplyBlock(nodeName, text string) error {
	if text == "" {
		return fmt.Errorf("empty block text")
	}
	n, err := w.lookup(nodeName)
	if err != nil {
		return err
	}
	if err := writableArchive(n); err != nil {
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
	n, err := w.lookup(nodeName)
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
	n, err := w.lookup(nodeName)
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
	for i := range w.Nodes {
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
