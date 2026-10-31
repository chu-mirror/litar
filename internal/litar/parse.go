package litar

import (
	"fmt"
	"strings"
)

// ParseError is a scan or parse failure.
// Archive errors name the file and line.
// Command-line errors are marked Expr and read "… in expression".
type ParseError struct {
	Msg  string
	File string
	Line int
	Expr bool
}

func (e *ParseError) Error() string {
	if e.Expr {
		return e.Msg + " in expression"
	}
	if e.File != "" {
		return fmt.Sprintf("%s at line %d in '%s'", e.Msg, e.Line, e.File)
	}
	return e.Msg
}

type parser struct {
	src  []byte
	i    int
	file string
	expr bool
}

func (p *parser) line() int {
	line := 1
	end := p.i
	if end > len(p.src) {
		end = len(p.src)
	}
	for i := 0; i < end; i++ {
		if p.src[i] == '\n' {
			line++
		}
	}
	return line
}

func (p *parser) fail(msg string) error {
	if p.expr {
		return &ParseError{Msg: msg, Expr: true}
	}
	return &ParseError{Msg: msg, File: p.file, Line: p.line()}
}

func isLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isWS(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// atControl reports a control at p.i.
// n == 1 is a final "@" at end of input.
// n == 2 is "@" plus the returned kind.
// "@" followed by a letter or a byte >= 128 is text, not a control.
func (p *parser) atControl() (kind byte, n int, ok bool) {
	if p.i >= len(p.src) || p.src[p.i] != '@' {
		return 0, 0, false
	}
	if p.i+1 >= len(p.src) {
		return 0, 1, true
	}
	next := p.src[p.i+1]
	if isLetter(next) || next >= 128 {
		return 0, 0, false
	}
	return next, 2, true
}

func isEndControl(kind byte, n int) bool {
	if n == 1 {
		return true
	}
	if n != 2 {
		return false
	}
	return kind == ' ' || kind == '\t' || kind == '\n' || kind == '\r'
}

func ctrlText(kind byte, n int) string {
	if n == 1 {
		return "@"
	}
	return "@" + string(rune(kind))
}

func (p *parser) unexpected(kind byte, n int) error {
	return p.fail(fmt.Sprintf("unexpected control '%s'", ctrlText(kind, n)))
}

func trimWS(b []byte) []byte {
	i := 0
	j := len(b)
	for i < j && isWS(b[i]) {
		i++
	}
	for j > i && isWS(b[j-1]) {
		j--
	}
	return b[i:j]
}

// decodeAt replaces "@@" with "@". The caller trims first.
func decodeAt(b []byte) string {
	var sb strings.Builder
	for i := 0; i < len(b); i++ {
		if b[i] == '@' && i+1 < len(b) && b[i+1] == '@' {
			sb.WriteByte('@')
			i++
			continue
		}
		sb.WriteByte(b[i])
	}
	return sb.String()
}

// readName reads bytes up to the next control.
// "@@" stays inside the name and decodes to one "@".
// Trim happens before that decode.
// allowEOF lets a command-line name end at end of input.
// kind, n describe the control that stopped the name; they are zero at EOF.
func (p *parser) readName(allowEOF bool) (Name, byte, int, error) {
	start := p.i
	for p.i < len(p.src) {
		kind, n, ok := p.atControl()
		if !ok {
			p.i++
			continue
		}
		if n == 2 && kind == '@' {
			p.i += 2
			continue
		}
		raw := p.src[start:p.i]
		return Name(decodeAt(trimWS(raw))), kind, n, nil
	}
	if !allowEOF {
		return "", 0, 0, p.fail("unterminated name")
	}
	return Name(decodeAt(trimWS(p.src[start:p.i]))), 0, 0, nil
}

type qOpt struct {
	dotOK    bool
	allowEOF bool
}

// parseQName reads labels, a module path, and a chunk.
// The terminating control is not consumed.
// An empty chunk is returned to the caller; it is not an error here.
func (p *parser) parseQName(opt qOpt) (ChunkRef, byte, int, error) {
	var labels []Name
	var module ModulePath
	moduleStarted := false

	name, kind, n, err := p.readName(opt.allowEOF)
	if err != nil {
		return ChunkRef{}, 0, 0, err
	}
	for {
		if kind == ':' && n == 2 && !moduleStarted {
			if name == "" || (!opt.dotOK && isRelName(name)) {
				if name == "" {
					return ChunkRef{}, 0, 0, p.fail("empty name")
				}
				return ChunkRef{}, 0, 0, p.fail("relative module name")
			}
			labels = append(labels, name)
			p.i += n
			name, kind, n, err = p.readName(opt.allowEOF)
			if err != nil {
				return ChunkRef{}, 0, 0, err
			}
			continue
		}
		if kind == ':' && n == 2 && moduleStarted {
			return ChunkRef{}, 0, 0, p.fail("label after module name")
		}
		if kind == '/' && n == 2 {
			if name == "" {
				// A leading @/ names the root explicitly.
				if !moduleStarted && len(module) == 0 {
					moduleStarted = true
					p.i += n
					name, kind, n, err = p.readName(opt.allowEOF)
					if err != nil {
						return ChunkRef{}, 0, 0, err
					}
					continue
				}
				return ChunkRef{}, 0, 0, p.fail("empty name")
			}
			if !opt.dotOK && isRelName(name) {
				return ChunkRef{}, 0, 0, p.fail("relative module name")
			}
			moduleStarted = true
			module = append(module, name)
			p.i += n
			name, kind, n, err = p.readName(opt.allowEOF)
			if err != nil {
				return ChunkRef{}, 0, 0, err
			}
			continue
		}
		break
	}
	if name != "" && !opt.dotOK && isRelName(name) {
		return ChunkRef{}, 0, 0, p.fail("relative module name")
	}
	return ChunkRef{Labels: labels, Module: module, Chunk: name}, kind, n, nil
}

func isViewOp(kind byte, n int) bool {
	return n == 2 && (kind == '+' || kind == '-' || kind == ',' || kind == '!')
}

func viewKind(kind byte) ViewOpKind {
	switch kind {
	case '+':
		return AddSet
	case '-':
		return SubSet
	case ',':
		return AddFile
	default:
		return DelFile
	}
}

// parseFilterList reads one or more filters. The caller has consumed the
// opening "@|". The terminating control is not consumed.
func (p *parser) parseFilterList(allowEOF bool) ([]Filter, byte, int, error) {
	var filters []Filter
	for {
		ref, kind, n, err := p.parseQName(qOpt{allowEOF: allowEOF})
		if err != nil {
			return nil, 0, 0, err
		}
		if ref.Chunk == "" {
			return nil, 0, 0, p.fail("empty name")
		}
		var view []ViewOp
		for isViewOp(kind, n) {
			op := kind
			p.i += n
			operand, k2, n2, err := p.readName(allowEOF)
			if err != nil {
				return nil, 0, 0, err
			}
			if operand == "" {
				return nil, 0, 0, p.fail("empty name")
			}
			view = append(view, ViewOp{Kind: viewKind(op), Name: operand})
			kind, n = k2, n2
		}
		filters = append(filters, Filter{Ref: ref, View: view})
		if kind == '|' && n == 2 {
			p.i += n
			continue
		}
		return filters, kind, n, nil
	}
}

func (p *parser) consumeLayout() {
	if p.i < len(p.src) && p.src[p.i] == '\r' {
		p.i++
		if p.i < len(p.src) && p.src[p.i] == '\n' {
			p.i++
		}
		return
	}
	if p.i < len(p.src) && p.src[p.i] == '\n' {
		p.i++
	}
}

// parseInnerRef reads a body reference. The caller has consumed "@<".
func (p *parser) parseInnerRef() (*Ref, error) {
	ref, kind, n, err := p.parseQName(qOpt{dotOK: true})
	if err != nil {
		return nil, err
	}
	var filters []Filter
	if kind == '|' && n == 2 {
		p.i += n
		filters, kind, n, err = p.parseFilterList(false)
		if err != nil {
			return nil, err
		}
	}
	if ref.Chunk == "" && len(filters) == 0 {
		return nil, p.fail("empty name")
	}
	if isEndControl(kind, n) {
		return nil, p.fail("unterminated block")
	}
	if kind != '>' || n != 2 {
		return nil, p.unexpected(kind, n)
	}
	p.i += n
	return &Ref{ChunkRef: ref, Filters: filters}, nil
}

func (p *parser) parseBody() ([]Part, []Filter, []Item, error) {
	var parts []Part
	var text strings.Builder
	flush := func() {
		if text.Len() == 0 {
			return
		}
		parts = append(parts, Part{Text: text.String()})
		text.Reset()
	}
	for {
		if p.i >= len(p.src) {
			return nil, nil, nil, p.fail("unterminated block")
		}
		kind, n, ok := p.atControl()
		if !ok {
			text.WriteByte(p.src[p.i])
			p.i++
			continue
		}
		if n == 2 && kind == '@' {
			text.WriteByte('@')
			p.i += 2
			continue
		}
		if isEndControl(kind, n) {
			flush()
			p.i += n
			return parts, nil, nil, nil
		}
		switch kind {
		case '<':
			flush()
			p.i += n
			ref, err := p.parseInnerRef()
			if err != nil {
				return nil, nil, nil, err
			}
			parts = append(parts, Part{Ref: ref})
		case '|':
			flush()
			p.i += n
			filters, k2, n2, err := p.parseFilterList(false)
			if err != nil {
				return nil, nil, nil, err
			}
			if k2 == '~' && n2 == 2 {
				p.i += n2
				items, err := p.parseItems()
				if err != nil {
					return nil, nil, nil, err
				}
				return parts, filters, items, nil
			}
			if isEndControl(k2, n2) {
				p.i += n2
				return parts, filters, nil, nil
			}
			return nil, nil, nil, p.unexpected(k2, n2)
		case '~':
			flush()
			p.i += n
			items, err := p.parseItems()
			if err != nil {
				return nil, nil, nil, err
			}
			return parts, nil, items, nil
		default:
			return nil, nil, nil, p.unexpected(kind, n)
		}
	}
}

func (p *parser) parseItems() ([]Item, error) {
	var items []Item
	for {
		var item Item
		for {
			name, kind, n, err := p.readName(false)
			if err != nil {
				return nil, err
			}
			if name == "" {
				return nil, p.fail("empty name")
			}
			item = append(item, name)
			if kind == '/' && n == 2 {
				p.i += n
				continue
			}
			items = append(items, item)
			if kind == '~' && n == 2 {
				p.i += n
				break
			}
			if isEndControl(kind, n) {
				p.i += n
				return items, nil
			}
			return nil, p.unexpected(kind, n)
		}
	}
}

func (p *parser) takeOp() (Op, error) {
	kind, n, ok := p.atControl()
	if !ok || n != 2 || (kind != '=' && kind != '+') {
		if !ok && p.i >= len(p.src) {
			return 0, p.fail("unterminated block")
		}
		if ok {
			return 0, p.unexpected(kind, n)
		}
		return 0, p.fail("unterminated block")
	}
	p.i += n
	return Op(kind), nil
}

func (p *parser) parseBlock() (Block, error) {
	startLine := p.line()
	// Caller is positioned at "@<".
	p.i += 2
	ref, kind, n, err := p.parseQName(qOpt{})
	if err != nil {
		return Block{}, err
	}
	if ref.Chunk == "" {
		return Block{}, p.fail("empty name")
	}
	if kind == '|' && n == 2 {
		return Block{}, p.unexpected(kind, n)
	}
	if n != 2 || (kind != '=' && kind != '+') {
		if n == 0 {
			return Block{}, p.fail("unterminated block")
		}
		return Block{}, p.unexpected(kind, n)
	}
	op := Op(kind)
	p.i += n
	p.consumeLayout()
	parts, filters, items, err := p.parseBody()
	if err != nil {
		return Block{}, err
	}
	return Block{
		Ref:       ref,
		Op:        op,
		Parts:     parts,
		Filters:   filters,
		Items:     items,
		StartLine: startLine,
		EndLine:   p.line(),
	}, nil
}

func (p *parser) parseFile(executable bool) (Block, error) {
	startLine := p.line()
	p.i += 2
	path, kind, n, err := p.readName(false)
	if err != nil {
		return Block{}, err
	}
	if path == "" {
		return Block{}, p.fail("empty name")
	}
	if kind == ':' && n == 2 {
		return Block{}, p.fail("files cannot be specialized")
	}
	if kind == '/' && n == 2 {
		return Block{}, p.fail("files cannot be modularized")
	}
	var sets []Name
	for kind == '`' && n == 2 {
		p.i += n
		setName, k2, n2, err := p.readName(false)
		if err != nil {
			return Block{}, err
		}
		if setName == "" {
			return Block{}, p.fail("empty name")
		}
		sets = append(sets, setName)
		kind, n = k2, n2
	}
	if kind == '+' && n == 2 {
		return Block{}, p.fail("files are paired only with @=")
	}
	if n != 2 || kind != '=' {
		if n == 0 {
			return Block{}, p.fail("unterminated block")
		}
		return Block{}, p.unexpected(kind, n)
	}
	op := Op(kind)
	p.i += n
	p.consumeLayout()
	parts, filters, items, err := p.parseBody()
	if err != nil {
		return Block{}, err
	}
	return Block{
		Ref:        ChunkRef{Chunk: path},
		Op:         op,
		Parts:      parts,
		Filters:    filters,
		Items:      items,
		StartLine:  startLine,
		EndLine:    p.line(),
		IsFile:     true,
		Path:       path,
		Executable: executable,
		Sets:       sets,
	}, nil
}

// skipToControl moves to the next control that starts or rejects an
// expression. "@@" stays in the comment.
func (p *parser) skipToControl() {
	for p.i < len(p.src) {
		kind, n, ok := p.atControl()
		if !ok {
			p.i++
			continue
		}
		if n == 2 && kind == '@' {
			p.i += 2
			continue
		}
		return
	}
}

// ParseArchive parses one archive. Comment+Raw of every block, then
// Trailing, reconstructs src.
func ParseArchive(src []byte, file string) (*Archive, error) {
	p := &parser{src: src, file: file}
	var blocks []Block
	for {
		commentStart := p.i
		p.skipToControl()
		if p.i >= len(p.src) {
			return &Archive{
				Blocks:   blocks,
				Trailing: string(src[commentStart:]),
				Source:   file,
			}, nil
		}
		kind, n, _ := p.atControl()
		comment := string(src[commentStart:p.i])
		rawStart := p.i
		var (
			b   Block
			err error
		)
		switch kind {
		case '<':
			b, err = p.parseBlock()
		case '[':
			b, err = p.parseFile(false)
		case '(':
			b, err = p.parseFile(true)
		default:
			return nil, p.unexpected(kind, n)
		}
		if err != nil {
			return nil, err
		}
		b.Comment = comment
		b.Raw = string(src[rawStart:p.i])
		b.Source = file
		b.EndLine = p.line()
		blocks = append(blocks, b)
	}
}

func (a *Archive) Text() string {
	var b strings.Builder
	for _, bl := range a.Blocks {
		b.WriteString(bl.Comment)
		b.WriteString(bl.Raw)
	}
	b.WriteString(a.Trailing)
	return b.String()
}

func parseExpr(s string, allowPipeline bool) (ChunkRef, []Filter, error) {
	p := &parser{src: []byte(s), expr: true}
	ref, kind, n, err := p.parseQName(qOpt{allowEOF: true})
	if err != nil {
		return ChunkRef{}, nil, err
	}
	var filters []Filter
	if kind == '|' && n == 2 {
		if !allowPipeline {
			return ChunkRef{}, nil, p.unexpected(kind, n)
		}
		p.i += n
		filters, kind, n, err = p.parseFilterList(true)
		if err != nil {
			return ChunkRef{}, nil, err
		}
	}
	if n != 0 || kind != 0 || p.i != len(p.src) {
		if n == 0 && p.i == len(p.src) {
			// EOF after the name.
		} else if isEndControl(kind, n) || n != 0 {
			return ChunkRef{}, nil, p.unexpected(kind, n)
		}
	}
	if ref.Chunk == "" && len(filters) == 0 {
		return ChunkRef{}, nil, p.fail("empty name")
	}
	// A pipeline may follow only when something terminated cleanly at EOF.
	if p.i != len(p.src) {
		return ChunkRef{}, nil, p.unexpected(kind, n)
	}
	return ref, filters, nil
}

// ParsePrint parses a -p expression: a qualified name and an optional pipeline.
func ParsePrint(s string) (ChunkRef, []Filter, error) {
	return parseExpr(s, true)
}

// ParseChunkRef parses a -l, -e, or -d operand. A pipeline is an error.
func ParseChunkRef(s string) (ChunkRef, error) {
	ref, filters, err := parseExpr(s, false)
	if err != nil {
		return ChunkRef{}, err
	}
	if len(filters) != 0 || ref.Chunk == "" {
		return ChunkRef{}, &ParseError{Msg: "empty name", Expr: true}
	}
	return ref, nil
}

// ParseModulePath parses a module path or a domain.
// "/" is the root. "." and ".." are rejected.
func ParseModulePath(s string) (ModulePath, error) {
	if s == "/" {
		return nil, nil
	}
	p := &parser{src: []byte(s), expr: true}
	var parts ModulePath
	for {
		name, kind, n, err := p.readName(true)
		if err != nil {
			return nil, err
		}
		if name == "" {
			return nil, p.fail("empty name")
		}
		if isRelName(name) {
			return nil, p.fail("relative module name")
		}
		parts = append(parts, name)
		if n == 0 {
			return parts, nil
		}
		if kind != '/' || n != 2 {
			return nil, p.unexpected(kind, n)
		}
		p.i += n
		if p.i >= len(p.src) {
			return nil, p.fail("empty name")
		}
	}
}

// ParseItem parses an -r operand. The root marker "/" is not special.
func ParseItem(s string) (Item, error) {
	if s == "" {
		return nil, &ParseError{Msg: "empty name", Expr: true}
	}
	p := &parser{src: []byte(s), expr: true}
	var item Item
	for {
		name, kind, n, err := p.readName(true)
		if err != nil {
			return nil, err
		}
		if name == "" {
			return nil, p.fail("empty name")
		}
		item = append(item, name)
		if n == 0 {
			return item, nil
		}
		if kind != '/' || n != 2 {
			return nil, p.unexpected(kind, n)
		}
		p.i += n
		if p.i >= len(p.src) {
			return nil, p.fail("empty name")
		}
	}
}
