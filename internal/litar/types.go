package litar

import (
	"fmt"
	"strings"
)

// A Name is a decoded, trimmed string. Interior whitespace stays.
type Name string

// ModulePath is empty for the root module.
type ModulePath []Name

type ChunkRef struct {
	Labels []Name
	Module ModulePath
	Chunk  Name // empty only on an invocation with a pipeline
}

type ViewOpKind int

const (
	AddSet ViewOpKind = iota
	SubSet
	AddFile
	DelFile
)

type ViewOp struct {
	Kind ViewOpKind
	Name Name
}

type Filter struct {
	Ref  ChunkRef
	View []ViewOp
}

// An Item is a path of names separated by @/.
type Item []Name

type Op byte

const (
	OpSet Op = '='
	OpAdd Op = '+'
)

type Part struct {
	Text string
	Ref  *Ref // set when this part is a chunk reference
}

type Ref struct {
	ChunkRef
	Filters []Filter
}

type Block struct {
	Ref       ChunkRef
	Op        Op
	Parts     []Part
	Filters   []Filter
	Items     []Item
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
}

type Node struct {
	Name        string
	Archive     string // resolved path
	ArchiveSpec string
	Description string
	Mappings    []Mapping
	Base        string // empty for the root
	Own         []Block
	Trailing    string
}

type World struct {
	Composer string
	Ground   *string // nil when this composer file has no ground
	Nodes    []Node  // ground nodes, then nodes declared in Composer
	localAt  int     // index of the first node declared in Composer
}

type Archive struct {
	Blocks   []Block
	Trailing string
	Source   string
}

// String renders a chunk reference in qualified-name form.
// The root module is omitted.
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

// key is a length-prefixed identity. Names may contain the bytes "@:".
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

// resolveModule applies "." and ".." against base when the path starts
// with one of them. Otherwise the path starts at the root, and a later
// "." or ".." still applies to the path built so far.
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

// matchRef reports whether a block header matches a chunk reference.
// The block's labels must be a suffix of the reference's labels.
// Empty block labels match every specialization.
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

// FormatBlocks is the -l form: each block's comment and raw text.
// Trailing bytes after the last expression are not included.
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
