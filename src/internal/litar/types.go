package litar

import (
	"fmt"
	"strings"
)

const (
	AddSet ViewOpKind = iota
	SubSet
	AddFile
	DelFile
)

const (
	OpSet Op = '='
	OpAdd Op = '+'
)

type Name string

type ModulePath []Name

type ChunkRef struct {
	Labels []Name
	Module ModulePath
	Chunk  Name // empty only on an invocation with a pipeline
}

type ViewOpKind int

type ViewOp struct {
	Kind ViewOpKind
	Name Name
}

type Filter struct {
	Ref  ChunkRef
	View []ViewOp
}

type Item []Name

type Op byte

type Part struct {
	Text string
	Ref  *Ref // set when this part is a chunk reference
}

type Ref struct {
	ChunkRef
	Filters []Filter
}

type Block struct {
	Ref     ChunkRef
	Op      Op
	Parts   []Part
	Filters []Filter
	// Items are all items the block carries; Defines are those
	// written after @?, which the block defines. The others it refers to.
	Items   []Item
	Defines []Item
	// EndsLine is set when a line break ended the content.
	// That line break is not content; an @+ block that has it
	// is joined to the text before it with a line break.
	EndsLine  bool
	Comment   string // bytes after the previous expression
	Raw       string // the expression as written, including the end control
	Source    string
	StartLine int
	EndLine   int
	// Set when the expression is @[ or @(.
	IsFile     bool
	Path       Name
	Executable bool
	Sets       []Name
}

type Mapping struct {
	Path       string
	FileSet    []string
	Origin     string // resolved against the composer file
	OriginSpec string // as written
	// ReadOnly is set on the mappings of the nodes of a ground:
	// filters read a mapped directory of the ground but never write it.
	ReadOnly bool
}

type Node struct {
	Name        string
	Archive     string // resolved path
	ArchiveSpec string
	Description string
	Mappings    []Mapping
	// Base is the path from the root down to the direct parent.
	// It is empty for the root.
	Base []string
	// Select lists the functional units the node selects, as written;
	// it is nil when the node has no select field.
	Select   []string
	Own      []Block
	Trailing string
}

type World struct {
	Composer    string
	Dir         string // absolute path of the composition directory
	Description string
	Ground      *string // nil when this composer file has no ground
	Nodes       []Node  // ground nodes, then nodes declared in Composer
	localAt     int     // index of the first node declared in Composer
}

type Archive struct {
	Blocks   []Block
	Trailing string
	Source   string
}

func (r ChunkRef) String() string {
	var b strings.Builder
	for _, label := range r.Labels {
		b.WriteString(string(label))
		b.WriteString("@:")
	}
	for i, part := range r.Module {
		if i > 0 {
			b.WriteString("@/")
		}
		b.WriteString(string(part))
	}
	if len(r.Module) > 0 {
		b.WriteString("@/")
	}
	b.WriteString(string(r.Chunk))
	return b.String()
}

func (r ChunkRef) key() string {
	var b strings.Builder
	writeNames(&b, r.Labels)
	b.WriteByte(0)
	writeNames(&b, r.Module)
	b.WriteByte(0)
	fmt.Fprintf(&b, "%d:", len(r.Chunk))
	b.WriteString(string(r.Chunk))
	return b.String()
}

func writeNames(b *strings.Builder, ns []Name) {
	for _, n := range ns {
		fmt.Fprintf(b, "%d:", len(n))
		b.WriteString(string(n))
		b.WriteByte(',')
	}
}

func modulesEqual(a, b ModulePath) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isRelName(n Name) bool {
	return n == "." || n == ".."
}

func hasRel(m ModulePath) bool {
	for _, n := range m {
		if isRelName(n) {
			return true
		}
	}
	return false
}

func resolveModule(base, path ModulePath) ModulePath {
	var cur ModulePath
	if len(path) > 0 && isRelName(path[0]) {
		cur = append(ModulePath(nil), base...)
	}
	for _, c := range path {
		switch c {
		case ".":
		case "..":
			if len(cur) > 0 {
				cur = cur[:len(cur)-1]
			}
		default:
			cur = append(cur, c)
		}
	}
	return cur
}

func copyLabels(ls []Name) []Name {
	if len(ls) == 0 {
		return nil
	}
	return append([]Name(nil), ls...)
}

func copyModule(m ModulePath) ModulePath {
	if len(m) == 0 {
		return nil
	}
	return append(ModulePath(nil), m...)
}

func matchRef(block, ref ChunkRef) bool {
	if block.Chunk != ref.Chunk || !modulesEqual(block.Module, ref.Module) {
		return false
	}
	return isSuffix(block.Labels, ref.Labels)
}

func isSuffix(block, ref []Name) bool {
	if len(block) > len(ref) {
		return false
	}
	off := len(ref) - len(block)
	for i := range block {
		if block[i] != ref[off+i] {
			return false
		}
	}
	return true
}

func formatModule(m ModulePath) string {
	if len(m) == 0 {
		return "/"
	}
	parts := make([]string, len(m))
	for i, n := range m {
		parts[i] = string(n)
	}
	return strings.Join(parts, "@/")
}

func formatItem(it Item) string {
	parts := make([]string, len(it))
	for i, n := range it {
		parts[i] = string(n)
	}
	return strings.Join(parts, "@/")
}

func sameItem(a, b Item) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func namePrefix(path []Name, prefix []Name) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

func FormatBlocks(blocks []Block) string {
	var b strings.Builder
	for _, bl := range blocks {
		b.WriteString(bl.Comment)
		b.WriteString(bl.Raw)
	}
	return b.String()
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func nodePath(n Node) []string {
	p := make([]string, len(n.Base)+1)
	copy(p, n.Base)
	p[len(n.Base)] = n.Name
	return p
}

func formatNodePath(p []string) string {
	return strings.Join(p, "/")
}

func samePath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasPathPrefix(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

func copyPath(p []string) []string {
	if len(p) == 0 {
		return []string{}
	}
	return append([]string{}, p...)
}
