package litar

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	spanHeader   spanRole = iota // the qualified name of a block header
	spanRef                      // a reference in content
	spanFilter                   // a filter of a pipeline
	spanPath                     // the path of a file header
	spanViewFile                 // a file operand of a view
	spanItem                     // an item
)

type spanRole int

// A nameSpan is one name of a block that a rewrite may replace.
// The bytes Raw[start:end] hold it as written, with the blanks around it.
type nameSpan struct {
	start, end int
	role       spanRole
	ref        ChunkRef // the qualified name of a header, reference, or filter
	name       Name     // the file path
	item       Item
}

// A spanScanner walks the text of one block and collects its names.
type spanScanner struct {
	p     parser
	spans []nameSpan
}

// A renaming moves a chunk, or every chunk of a module, to another path.
// With items set it moves an item, or every item of a domain.
type renaming struct {
	module bool
	items  bool
	// The module path or domain; a chunk or an item has its own name last.
	from, to []Name
}

// A fileMove sends a file, or every file below a directory, to another path.
type fileMove struct {
	from, to string
	dir      bool
}

type fileMoves []fileMove

// A fileIndex tells where each file visible in a node comes from.
type fileIndex struct {
	own map[string]bool // files of the node's own blocks
	// Files of the base nodes and mapped paths, with their source.
	// A mapped directory keeps its trailing slash.
	other map[string]string
}

// header reads up to the operation control and moves past it.
func (s *spanScanner) header() error {
	p := &s.p
	if len(p.src) < 2 {
		return p.fail("unterminated block")
	}
	file := p.src[1] != '<'
	p.i = 2
	start := p.i
	if !file {
		ref, _, n, err := p.parseQName(qOpt{})
		if err != nil {
			return err
		}
		s.spans = append(s.spans, nameSpan{start: start, end: p.i, role: spanHeader, ref: ref})
		p.i += n
		return nil
	}
	name, kind, n, err := p.readName(false)
	if err != nil {
		return err
	}
	s.spans = append(s.spans, nameSpan{start: start, end: p.i, role: spanPath, name: name})
	for kind == '`' && n == 2 {
		p.i += n
		if _, kind, n, err = p.readName(false); err != nil {
			return err
		}
	}
	p.i += n
	return nil
}

// body reads the content, the pipeline, and the items.
func (s *spanScanner) body() error {
	p := &s.p
	for p.i < len(p.src) {
		kind, n, ok := p.atControl()
		switch {
		case !ok:
			p.i++
		case n == 2 && kind == '@':
			p.i += 2
		case n == 2 && kind == '<':
			p.i += n
			if err := s.reference(); err != nil {
				return err
			}
		case n == 2 && kind == '|':
			p.i += n
			if err := s.filters(); err != nil {
				return err
			}
		case n == 2 && kind == '~':
			p.i += n
			return s.items()
		default:
			return nil
		}
	}
	return nil
}

// reference reads a reference in content. The caller has consumed "@<".
func (s *spanScanner) reference() error {
	p := &s.p
	start := p.i
	ref, kind, n, err := p.parseQName(qOpt{dotOK: true})
	if err != nil {
		return err
	}
	// An invocation of a pipeline alone has no chunk.
	if ref.Chunk != "" {
		s.spans = append(s.spans, nameSpan{start: start, end: p.i, role: spanRef, ref: ref})
	}
	if kind == '|' && n == 2 {
		p.i += n
		if err := s.filters(); err != nil {
			return err
		}
	}
	p.i += 2
	return nil
}

// filters reads a pipeline up to the control that ends it.
// Of the view operands, only files are names a rewrite replaces.
func (s *spanScanner) filters() error {
	p := &s.p
	for {
		start := p.i
		ref, kind, n, err := p.parseQName(qOpt{})
		if err != nil {
			return err
		}
		s.spans = append(s.spans, nameSpan{start: start, end: p.i, role: spanFilter, ref: ref})
		for isViewOp(kind, n) {
			op := viewKind(kind)
			p.i += n
			start = p.i
			var name Name
			if name, kind, n, err = p.readName(false); err != nil {
				return err
			}
			if op == AddFile || op == DelFile {
				s.spans = append(s.spans, nameSpan{start: start, end: p.i, role: spanViewFile, name: name})
			}
		}
		if kind != '|' || n != 2 {
			return nil
		}
		p.i += n
	}
}

// subtree lists the node and the local nodes based on it.
func (w *World) subtree(n *Node) []*Node {
	root := nodePath(*n)
	var nodes []*Node
	for i := w.localAt; i < len(w.Nodes); i++ {
		if hasPathPrefix(nodePath(w.Nodes[i]), root) {
			nodes = append(nodes, &w.Nodes[i])
		}
	}
	return nodes
}

// rewriteNodes applies edit to the own blocks of the nodes and compacts them.
// No archive is written unless every node could be rewritten.
func (w *World) rewriteNodes(nodes []*Node, edit func(Block, nameSpan) (string, bool)) error {
	type pending struct {
		n    *Node
		data []byte
		arch *Archive
	}
	var all []pending
	for _, n := range nodes {
		var b strings.Builder
		for _, bl := range n.Own {
			raw, err := rewriteRaw(bl, edit)
			if err != nil {
				return err
			}
			b.WriteString(bl.Comment)
			b.WriteString(raw)
		}
		b.WriteString(n.Trailing)
		arch, err := ParseArchive([]byte(b.String()), n.Archive)
		if err != nil {
			return err
		}
		renamed := *n
		renamed.Own, renamed.Trailing = arch.Blocks, arch.Trailing
		data, arch, err := compactOwn(&renamed)
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

// apply returns ref renamed. A description in the namespace Chunk moves with
// the module or chunk it describes, and one in Index with its domain or item;
// a renaming of items moves no other chunk.
func (r renaming) apply(ref ChunkRef) (ChunkRef, bool) {
	full := refPath(ref)
	skip := 0
	if len(ref.Module) > 0 && ref.Module[0] == r.namespace() {
		skip = 1
	}
	if r.items && skip == 0 {
		return ref, false
	}
	rest := full[skip:]
	// A module moves the chunks below it; its own path names only its description.
	exact := len(rest) == len(r.from) && (!r.module || skip == 1)
	below := r.module && len(rest) > len(r.from)
	if !(exact || below) || !namePrefix(rest, r.from) {
		return ref, false
	}
	moved := append([]Name{}, full[:skip]...)
	moved = append(moved, r.to...)
	moved = append(moved, rest[len(r.from):]...)
	ref.Module = ModulePath(moved[:len(moved)-1])
	ref.Chunk = moved[len(moved)-1]
	return ref, true
}

// edit replaces the names of a block that the renaming moves.
func (r renaming) edit(bl Block, sp nameSpan) (string, bool) {
	switch sp.role {
	case spanHeader, spanFilter:
		if to, ok := r.apply(sp.ref); ok {
			return encodeRef(to), true
		}
	case spanRef:
		return r.editRef(bl, sp.ref)
	case spanItem:
		to, ok := r.applyItem(sp.item)
		return encodeItem(to), ok
	}
	return "", false
}

// editRef renames a reference in the content of bl. A relative path stays as
// written while it still leads from the renamed header to the renamed chunk;
// otherwise the reference is written from the root.
func (r renaming) editRef(bl Block, ref ChunkRef) (string, bool) {
	abs := ref
	abs.Module = resolveModule(bl.Ref.Module, ref.Module)
	to, moved := r.apply(abs)
	if !hasRel(ref.Module) {
		return encodeRef(to), moved
	}
	head, _ := r.apply(bl.Ref)
	if to.Chunk == ref.Chunk && modulesEqual(resolveModule(head.Module, ref.Module), to.Module) {
		return "", false
	}
	return encodeRef(to), true
}

// what names the kind of the renaming in a message.
func (r renaming) what() string {
	switch {
	case r.items && r.module:
		return "domain"
	case r.items:
		return "item"
	case r.module:
		return "module"
	}
	return "chunk"
}

// moves reports whether the renaming moves the chunk of a block,
// leaving its description aside.
func (r renaming) moves(bl Block) bool {
	if len(bl.Ref.Module) > 0 && bl.Ref.Module[0] == "Chunk" {
		return false
	}
	_, ok := r.apply(bl.Ref)
	return ok
}

// validate refuses an empty path and, for chunks, the namespaces of descriptions.
func (r renaming) validate() error {
	if len(r.from) == 0 || len(r.to) == 0 {
		return fmt.Errorf("empty name")
	}
	if r.items {
		return nil
	}
	for _, p := range [][]Name{r.from, r.to} {
		if _, ok := reservedNamespace(ModulePath(p)); ok {
			return fmt.Errorf("'%s' is reserved", formatModule(ModulePath(p)))
		}
	}
	return nil
}

// baseChunks lists in taken the chunks of the base nodes of n.
// A base keeps its blocks, so a chunk it defines is not renamed from n.
func (w *World) baseChunks(n *Node, r renaming, taken map[string]bool) error {
	rev, err := w.ancestry(formatNodePath(nodePath(*n)))
	if err != nil {
		return err
	}
	for _, base := range rev[1:] {
		for _, bl := range base.Own {
			if r.moves(bl) {
				return fmt.Errorf("%s '%s' is defined in node '%s'", r.what(), formatModule(ModulePath(r.from)), base.Name)
			}
			taken[chunkKey(bl.Ref)] = true
		}
	}
	return nil
}

// checkRename refuses a renaming that would split a chunk or join two:
// the node itself must define what is renamed, and no other chunk may
// hold a new path.
func (w *World) checkRename(n *Node, r renaming) error {
	if err := r.validate(); err != nil {
		return err
	}
	if r.items {
		return w.checkItems(n, r)
	}
	name := formatModule(ModulePath(r.from))
	taken := map[string]bool{}
	if err := w.baseChunks(n, r, taken); err != nil {
		return err
	}
	var moved []ChunkRef
	found := false
	for _, sub := range w.subtree(n) {
		for _, bl := range sub.Own {
			to, ok := r.apply(bl.Ref)
			if !ok {
				taken[chunkKey(bl.Ref)] = true
				continue
			}
			if bl.IsFile {
				return fmt.Errorf("'%s' is a file", name)
			}
			moved = append(moved, to)
			found = found || sub == n && r.moves(bl)
		}
	}
	if !found {
		return fmt.Errorf("unknown %s '%s'", r.what(), name)
	}
	for _, to := range moved {
		if taken[chunkKey(to)] {
			return fmt.Errorf("chunk '%s' already exists", ChunkRef{Module: to.Module, Chunk: to.Chunk})
		}
	}
	return nil
}

// Rename moves a chunk, a module, an item, or a domain to another path.
// Block headers, references, filters, items, and descriptions follow,
// in the node and in the local nodes based on it.
func (w *World) Rename(nodeName string, r renaming) error {
	n, err := w.requireLocal(nodeName)
	if err != nil {
		return err
	}
	if err := w.checkRename(n, r); err != nil {
		return err
	}
	return w.rewriteNodes(w.subtree(n), r.edit)
}

// apply returns the new path of a file, or of a directory that moves.
func (m fileMove) apply(p string) (string, bool) {
	if p == m.from {
		return m.to, true
	}
	if m.dir && strings.HasPrefix(p, m.from+"/") {
		return m.to + p[len(m.from):], true
	}
	return p, false
}

func (ms fileMoves) apply(p string) (string, bool) {
	for _, m := range ms {
		if to, ok := m.apply(p); ok {
			return to, true
		}
	}
	return p, false
}

// applyRef moves the root chunk of a file and the description of a file
// or directory, File@/dir@/file.
func (ms fileMoves) applyRef(ref ChunkRef) (ChunkRef, bool) {
	if len(ref.Module) == 0 {
		to, ok := ms.apply(string(ref.Chunk))
		ref.Chunk = Name(to)
		return ref, ok
	}
	if ref.Module[0] != "File" {
		return ref, false
	}
	var parts []string
	for _, n := range refPath(ref)[1:] {
		parts = append(parts, string(n))
	}
	to, ok := ms.apply(strings.Join(parts, "/"))
	if !ok {
		return ref, false
	}
	parts = strings.Split(to, "/")
	ref.Module = ModulePath{"File"}
	for _, part := range parts[:len(parts)-1] {
		ref.Module = append(ref.Module, Name(part))
	}
	ref.Chunk = Name(parts[len(parts)-1])
	return ref, true
}

// edit replaces the names of a block that the moves change: file headers,
// file operands of views, and the names that applyRef moves.
func (ms fileMoves) edit(bl Block, sp nameSpan) (string, bool) {
	switch sp.role {
	case spanItem:
		return "", false
	case spanPath, spanViewFile:
		to, ok := ms.apply(string(sp.name))
		return encodeName(Name(to)), ok
	}
	ref := sp.ref
	if sp.role == spanRef {
		ref.Module = resolveModule(bl.Ref.Module, ref.Module)
	}
	to, ok := ms.applyRef(ref)
	return encodeRef(to), ok
}

// indexFiles collects the files visible in the node.
func (w *World) indexFiles(n *Node) (fileIndex, error) {
	ix := fileIndex{own: map[string]bool{}, other: map[string]string{}}
	rev, err := w.ancestry(formatNodePath(nodePath(*n)))
	if err != nil {
		return ix, err
	}
	for i, node := range rev {
		for _, m := range node.Mappings {
			ix.other[m.Path] = "a mapping"
		}
		for _, bl := range node.Own {
			switch {
			case !bl.IsFile:
			case i == 0:
				ix.own[string(bl.Path)] = true
			default:
				ix.other[string(bl.Path)] = fmt.Sprintf("node '%s'", node.Name)
			}
		}
	}
	return ix, nil
}

// under lists the own files at p or below the directory p.
func (ix fileIndex) under(p string) []string {
	var files []string
	for f := range ix.own {
		if f == p || strings.HasPrefix(f, p+"/") {
			files = append(files, f)
		}
	}
	sort.Strings(files)
	return files
}

// foreign names the source of a file at or below p that the node does not
// hold itself, or of a mapped directory that p lies in.
func (ix fileIndex) foreign(p string) (string, bool) {
	var keys []string
	for k := range ix.other {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f := strings.TrimSuffix(k, "/")
		inside := strings.HasSuffix(k, "/") && strings.HasPrefix(p, k)
		if f == p || strings.HasPrefix(f, p+"/") || inside {
			return ix.other[k], true
		}
	}
	return "", false
}

// isDir reports whether some visible file lies below p.
func (ix fileIndex) isDir(p string) bool {
	for f := range ix.own {
		if strings.HasPrefix(f, p+"/") {
			return true
		}
	}
	for f := range ix.other {
		if strings.HasPrefix(f, p+"/") {
			return true
		}
	}
	return false
}

// source resolves an operand to the own files it names.
// dir is set when the operand is a directory.
func (ix fileIndex) source(arg string) (p string, files []string, dir bool, err error) {
	if p, err = cleanPath(arg); err != nil {
		return "", nil, false, err
	}
	if where, ok := ix.foreign(p); ok {
		return "", nil, false, fmt.Errorf("'%s' comes from %s", p, where)
	}
	files = ix.under(p)
	return p, files, len(files) > 0 && !ix.own[p], nil
}

// OperateFiles interprets one rm or mv command on the files of the node,
// and returns a line for each file it changed.
func (w *World) OperateFiles(nodeName, command string) (string, error) {
	n, err := w.requireLocal(nodeName)
	if err != nil {
		return "", err
	}
	words, err := shellWords(command)
	if err != nil {
		return "", err
	}
	if len(words) == 0 {
		return "", fmt.Errorf("empty command")
	}
	ix, err := w.indexFiles(n)
	if err != nil {
		return "", err
	}
	var lines []string
	switch words[0] {
	case "rm":
		lines, err = w.removeFiles(n, ix, words[1:])
	case "mv":
		lines, err = w.moveFiles(n, ix, words[1:])
	default:
		err = fmt.Errorf("unsupported command '%s'", words[0])
	}
	return joinLines(lines), err
}

// removeFiles is rm: it deletes the own blocks of each file, the blocks
// applied to its root chunk, and its description. A directory needs -r.
func (w *World) removeFiles(n *Node, ix fileIndex, words []string) ([]string, error) {
	flags, args, err := fileArgs(words, "rRf")
	if err != nil {
		return nil, err
	}
	recursive := strings.ContainsAny(flags, "rR")
	var gone fileMoves
	var lines []string
	kept := n.Mappings
	for _, arg := range args {
		if rest, ok := unmap(kept, arg); ok {
			kept = rest
			lines = append(lines, "unmapped "+path.Clean(arg))
			continue
		}
		p, files, dir, err := ix.source(arg)
		switch {
		case err != nil:
			return nil, err
		case len(files) == 0 && strings.Contains(flags, "f"):
			continue
		case len(files) == 0:
			return nil, fmt.Errorf("no such file '%s'", p)
		case dir && !recursive:
			return nil, fmt.Errorf("'%s' is a directory", p)
		}
		gone = append(gone, fileMove{from: p, dir: dir})
		for _, f := range files {
			lines = append(lines, "removed "+f)
		}
	}
	if len(gone) > 0 {
		err = w.rewriteOwn(n, func(b Block, _ int) bool {
			_, ok := gone.applyRef(ChunkRef{Module: b.Ref.Module, Chunk: b.Ref.Chunk})
			return !ok
		})
	}
	if err == nil && len(kept) != len(n.Mappings) {
		saved := n.Mappings
		n.Mappings = kept
		if err = w.save(); err != nil {
			n.Mappings = saved
			err = fmt.Errorf("could not write '%s'", w.Composer)
		}
	}
	return lines, err
}

// moveTarget reads the last operand of mv. It is the new path, or the
// directory that receives the sources when it holds files, ends with a slash,
// or follows several sources.
func (ix fileIndex) moveTarget(args []string) (target string, into bool, err error) {
	if len(args) < 2 {
		return "", false, fmt.Errorf("mv needs a source and a target")
	}
	last := args[len(args)-1]
	if target, err = cleanPath(last); err != nil {
		return "", false, err
	}
	into = ix.isDir(target) || strings.HasSuffix(last, "/")
	if len(args) > 2 && !into {
		return "", false, fmt.Errorf("target '%s' is not a directory", target)
	}
	return target, into, nil
}

// planMoves turns the sources of mv into moves.
func (ix fileIndex) planMoves(sources []string, target string, into bool) (fileMoves, error) {
	var moves fileMoves
	for _, arg := range sources {
		p, files, dir, err := ix.source(arg)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("no such file '%s'", p)
		}
		m := fileMove{from: p, to: target, dir: dir}
		if into {
			m.to = path.Join(target, path.Base(p))
		}
		if dir && strings.HasPrefix(m.to+"/", p+"/") {
			return nil, fmt.Errorf("cannot move '%s' into itself", p)
		}
		moves = append(moves, m)
	}
	return moves, nil
}

// settle lists each file that moves with its new path.
// A file is never moved onto another one, unlike the mv of a shell,
// because the block of the other file would be lost without a trace.
func (ix fileIndex) settle(moves fileMoves) ([]string, error) {
	moving := map[string]string{}
	var order []string
	for _, m := range moves {
		for _, f := range ix.under(m.from) {
			if _, ok := moving[f]; !ok {
				moving[f], _ = m.apply(f)
				order = append(order, f)
			}
		}
	}
	var lines []string
	taken := map[string]bool{}
	for _, f := range order {
		to := moving[f]
		_, leaves := moving[to]
		_, foreign := ix.foreign(to)
		if taken[to] || foreign || ix.own[to] && !leaves || ix.isDir(to) {
			return nil, fmt.Errorf("file '%s' already exists", to)
		}
		taken[to] = true
		lines = append(lines, f+" -> "+to)
	}
	return lines, nil
}

// moveFiles is mv: file headers, references to the files, file operands of
// views, and descriptions follow, in the node and the local nodes based on it.
func (w *World) moveFiles(n *Node, ix fileIndex, words []string) ([]string, error) {
	_, args, err := fileArgs(words, "f")
	if err != nil {
		return nil, err
	}
	target, into, err := ix.moveTarget(args)
	if err != nil {
		return nil, err
	}
	moves, err := ix.planMoves(args[:len(args)-1], target, into)
	if err != nil {
		return nil, err
	}
	lines, err := ix.settle(moves)
	if err != nil {
		return nil, err
	}
	return lines, w.rewriteNodes(w.subtree(n), moves.edit)
}

// items reads the items up to the end control. The caller has consumed "@~".
func (s *spanScanner) items() error {
	p := &s.p
	start := p.i
	var item Item
	for {
		name, kind, n, err := p.readName(false)
		if err != nil {
			return err
		}
		item = append(item, name)
		if kind == '/' && n == 2 {
			p.i += n
			continue
		}
		s.spans = append(s.spans, nameSpan{start: start, end: p.i, role: spanItem, item: item})
		if kind != '~' || n != 2 {
			return nil
		}
		p.i += n
		start, item = p.i, nil
	}
}

// namespace is the reserved module that holds the descriptions of what moves.
func (r renaming) namespace() Name {
	if r.items {
		return "Index"
	}
	return "Chunk"
}

// applyItem returns an item renamed.
func (r renaming) applyItem(it Item) (Item, bool) {
	exact := len(it) == len(r.from) && !r.module
	below := r.module && len(it) > len(r.from)
	if !r.items || !(exact || below) || !namePrefix(it, r.from) {
		return it, false
	}
	return append(append(Item{}, r.to...), it[len(r.from):]...), true
}

// carried reports whether a block carries an item that the renaming moves.
func (r renaming) carried(bl Block) bool {
	for _, it := range bl.Items {
		if _, ok := r.applyItem(it); ok {
			return true
		}
	}
	return false
}

// checkItems refuses to rename an item that the node does not carry, or that
// a base node carries: the base keeps its blocks, so the cross-reference
// would break in two.
func (w *World) checkItems(n *Node, r renaming) error {
	name := formatModule(ModulePath(r.from))
	rev, err := w.ancestry(formatNodePath(nodePath(*n)))
	if err != nil {
		return err
	}
	for _, base := range rev[1:] {
		for _, bl := range base.Own {
			if r.carried(bl) {
				return fmt.Errorf("%s '%s' is used in node '%s'", r.what(), name, base.Name)
			}
		}
	}
	for _, bl := range n.Own {
		if r.carried(bl) {
			return nil
		}
	}
	return fmt.Errorf("unknown %s '%s'", r.what(), name)
}

// MapFile adds a mapping to a node: the file or directory origin, outside
// the archives, appears in the node at path.
func (w *World) MapFile(nodeName, at, origin string, sets []string) error {
	n, err := w.requireLocal(nodeName)
	if err != nil {
		return err
	}
	p, err := cleanPath(at)
	if err != nil {
		return err
	}
	if origin == "" {
		return fmt.Errorf("missing origin")
	}
	if _, ok := unmap(n.Mappings, p); ok {
		return fmt.Errorf("mapping '%s' already exists", p)
	}
	if strings.HasSuffix(at, "/") {
		p += "/"
	}
	m := Mapping{Path: p, FileSet: append([]string{}, sets...), Origin: origin, OriginSpec: origin}
	if !filepath.IsAbs(origin) {
		m.Origin = filepath.Join(filepath.Dir(w.Composer), origin)
	}
	n.Mappings = append(n.Mappings, m)
	if err := w.save(); err != nil {
		n.Mappings = n.Mappings[:len(n.Mappings)-1]
		return fmt.Errorf("could not write '%s'", w.Composer)
	}
	return nil
}

// OverrideBlock puts the blocks of text in the place of one block of the node.
// The new blocks are applied to the chunk reference of the old one, so the
// change stays within one chunk; the comment of the old block goes with it.
func (w *World) OverrideBlock(nodeName, ref, text string) error {
	n, err := w.requireLocal(nodeName)
	if err != nil {
		return err
	}
	i, err := ownBlock(n, ref)
	if err != nil {
		return err
	}
	arch, err := ParseArchive([]byte(text), n.Archive)
	if err != nil {
		return err
	}
	if len(arch.Blocks) == 0 {
		return fmt.Errorf("empty block text")
	}
	for _, bl := range arch.Blocks {
		if bl.Ref.key() != n.Own[i].Ref.key() || bl.IsFile != n.Own[i].IsFile {
			return fmt.Errorf("block '%s' is not applied to '%s'", bl.Ref, n.Own[i].Ref)
		}
	}
	// The new text starts where the comment of the old block started.
	blank := n.Own[i].Comment
	blank = blank[:len(blank)-len(strings.TrimLeft(blank, " \t\r\n"))]
	arch.Blocks[0].Comment = blank + strings.TrimLeft(arch.Blocks[0].Comment, " \t\r\n")
	var b strings.Builder
	joinBlocks(&b, n.Own[:i])
	joinBlocks(&b, arch.Blocks)
	joinBlocks(&b, n.Own[i+1:])
	b.WriteString(n.Trailing)
	data, next, err := compacted(n, b.String())
	if err != nil {
		return err
	}
	return store(n, data, next)
}

// DeleteBlock removes one block of the node.
func (w *World) DeleteBlock(nodeName, ref string) error {
	n, err := w.requireLocal(nodeName)
	if err != nil {
		return err
	}
	i, err := ownBlock(n, ref)
	if err != nil {
		return err
	}
	return w.rewriteOwn(n, func(_ Block, at int) bool { return at != i })
}

// MoveBlock takes one block of a node, with its comment, to the end of
// another local node. Neither archive is written unless both can be.
func (w *World) MoveBlock(nodeName, ref, toName string) error {
	n, err := w.requireLocal(nodeName)
	if err != nil {
		return err
	}
	to, err := w.requireLocal(toName)
	if err != nil {
		return err
	}
	if n == to {
		return fmt.Errorf("block '%s' is already in node '%s'", ref, to.Name)
	}
	i, err := ownBlock(n, ref)
	if err != nil {
		return err
	}
	var from, into strings.Builder
	joinBlocks(&from, n.Own[:i])
	joinBlocks(&from, n.Own[i+1:])
	from.WriteString(n.Trailing)
	joinBlocks(&into, to.Own)
	joinBlocks(&into, n.Own[i:i+1])
	into.WriteString(to.Trailing)
	fromData, fromArch, err := compacted(n, from.String())
	if err != nil {
		return err
	}
	intoData, intoArch, err := compacted(to, into.String())
	if err != nil {
		return err
	}
	if err := store(to, intoData, intoArch); err != nil {
		return err
	}
	return store(n, fromData, fromArch)
}

// scanSpans lists the names of a block that parsed before, in order.
func scanSpans(raw string) ([]nameSpan, error) {
	s := &spanScanner{p: parser{src: []byte(raw)}}
	if err := s.header(); err != nil {
		return nil, err
	}
	if err := s.body(); err != nil {
		return nil, err
	}
	return s.spans, nil
}

// encodeName writes a name as an archive holds it.
func encodeName(n Name) string {
	return strings.ReplaceAll(string(n), "@", "@@")
}

// encodeRef writes a qualified name as an archive holds it.
func encodeRef(r ChunkRef) string {
	var b strings.Builder
	for _, label := range r.Labels {
		b.WriteString(encodeName(label) + "@:")
	}
	for _, part := range r.Module {
		b.WriteString(encodeName(part) + "@/")
	}
	b.WriteString(encodeName(r.Chunk))
	return b.String()
}

// refPath is the module path of ref followed by its chunk.
func refPath(ref ChunkRef) []Name {
	return append(append([]Name{}, ref.Module...), ref.Chunk)
}

// rewriteRaw is the text of a block with the names that edit replaces.
// The spans are replaced from the last one, so the earlier offsets stay valid,
// and a new name keeps the blanks that surrounded the old one.
func rewriteRaw(bl Block, edit func(Block, nameSpan) (string, bool)) (string, error) {
	spans, err := scanSpans(bl.Raw)
	if err != nil {
		return "", err
	}
	raw := bl.Raw
	for i := len(spans) - 1; i >= 0; i-- {
		sp := spans[i]
		if text, ok := edit(bl, sp); ok {
			old := raw[sp.start:sp.end]
			inner := strings.TrimLeft(old, " \t\r\n")
			lead := old[:len(old)-len(inner)]
			trail := inner[len(strings.TrimRight(inner, " \t\r\n")):]
			raw = raw[:sp.start] + lead + text + trail + raw[sp.end:]
		}
	}
	return raw, nil
}

// shellWords splits a command into words the way a shell splits plain words:
// blanks separate them, and quotes or a backslash keep the next bytes literal.
func shellWords(cmd string) ([]string, error) {
	var words []string
	var word strings.Builder
	open := false
	quote := byte(0)
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote == '\'', quote == '"' && c != '\\':
			word.WriteByte(c)
		case c == '\\':
			if i+1 == len(cmd) {
				return nil, fmt.Errorf("unterminated escape in command")
			}
			i++
			word.WriteByte(cmd[i])
			open = true
		case c == '\'' || c == '"':
			quote = c
			open = true
		case isWS(c):
			if open {
				words = append(words, word.String())
				word.Reset()
				open = false
			}
		default:
			word.WriteByte(c)
			open = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in command")
	}
	if open {
		words = append(words, word.String())
	}
	return words, nil
}

// fileArgs separates the option letters of a command from its operands.
// Only the letters of allowed are options of the command.
func fileArgs(words []string, allowed string) (flags string, args []string, err error) {
	i := 0
	for ; i < len(words); i++ {
		w := words[i]
		if w == "--" {
			i++
			break
		}
		if len(w) < 2 || w[0] != '-' {
			break
		}
		for _, c := range w[1:] {
			if !strings.ContainsRune(allowed, c) {
				return "", nil, fmt.Errorf("unsupported option '%s'", w)
			}
		}
		flags += w[1:]
	}
	return flags, words[i:], nil
}

// cleanPath is the path of an operand as a file header writes it.
func cleanPath(arg string) (string, error) {
	p := path.Clean(arg)
	if arg == "" || p == "." || p == ".." || strings.HasPrefix(p, "../") || path.IsAbs(p) {
		return "", fmt.Errorf("invalid path '%s'", arg)
	}
	return p, nil
}

// unmap takes the mapping of the path of arg away from maps.
func unmap(maps []Mapping, arg string) ([]Mapping, bool) {
	p := path.Clean(arg)
	for i, m := range maps {
		if strings.TrimSuffix(m.Path, "/") == p {
			return append(append([]Mapping{}, maps[:i]...), maps[i+1:]...), true
		}
	}
	return maps, false
}

// ownBlock finds a block of the node by its ref.
func ownBlock(n *Node, ref string) (int, error) {
	for i, bl := range n.Own {
		if blockRef(bl) == ref {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unknown block '%s'", ref)
}

// joinBlocks writes blocks as an archive holds them.
// A block that closed at the end of its archive has no blank after its "@";
// the next text would join it.
func joinBlocks(b *strings.Builder, blocks []Block) {
	for _, bl := range blocks {
		b.WriteString(bl.Comment)
		b.WriteString(bl.Raw)
		if !endsWithSpace(bl.Raw) {
			b.WriteByte('\n')
		}
	}
}

// compacted parses text as the archive of n and compacts it, writing nothing.
func compacted(n *Node, text string) ([]byte, *Archive, error) {
	arch, err := ParseArchive([]byte(text), n.Archive)
	if err != nil {
		return nil, nil, err
	}
	next := *n
	next.Own, next.Trailing = arch.Blocks, arch.Trailing
	return compactOwn(&next)
}

// store writes a compacted archive and makes it the content of the node.
func store(n *Node, data []byte, arch *Archive) error {
	if err := writeFileKeepMode(n.Archive, data); err != nil {
		return fmt.Errorf("could not write '%s'", n.Archive)
	}
	n.Own, n.Trailing = arch.Blocks, arch.Trailing
	return nil
}

// encodeItem writes an item as an archive holds it.
func encodeItem(it Item) string {
	parts := make([]string, len(it))
	for i, n := range it {
		parts[i] = encodeName(n)
	}
	return strings.Join(parts, "@/")
}
