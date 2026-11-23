package litar

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type namedLine struct {
	name string
	desc string
}

type unit struct {
	chunk *ChunkRef
	item  Item
	file  string
}

func matchingBlocks(blocks []Block, ref ChunkRef) []Block {
	var out []Block
	for _, b := range blocks {
		if matchRef(b.Ref, ref) {
			out = append(out, b)
		}
	}
	return out
}

func effectiveBlocks(blocks []Block, ref ChunkRef) (eff []Block, ok bool) {
	for _, b := range blocks {
		if !matchRef(b.Ref, ref) {
			continue
		}
		ok = true
		if b.Op == OpSet {
			eff = []Block{b}
		} else {
			eff = append(eff, b)
		}
	}
	return eff, ok
}

func Literal(blocks []Block, ref ChunkRef) (string, error) {
	got := matchingBlocks(blocks, ref)
	if len(got) == 0 {
		return "", fmt.Errorf("chunk '%s' is not defined", ref.String())
	}
	return FormatBlocks(got), nil
}

func Effective(blocks []Block, ref ChunkRef) (string, error) {
	got, ok := effectiveBlocks(blocks, ref)
	if !ok {
		return "", fmt.Errorf("chunk '%s' is not defined", ref.String())
	}
	return FormatBlocks(got), nil
}

func useMatches(use ChunkRef, query ChunkRef, header ModulePath, outer []Name, open bool) bool {
	mod := use.Module
	if hasRel(mod) {
		mod = resolveModule(header, mod)
	}
	if use.Chunk != query.Chunk || !modulesEqual(mod, query.Module) {
		return false
	}
	if len(use.Labels) > 0 {
		return isSuffix(query.Labels, use.Labels)
	}
	return isSuffix(query.Labels, outer) || open && isSuffix(outer, query.Labels)
}

func blockDepends(b Block, query ChunkRef) bool {
	check := func(use ChunkRef, outer []Name, open bool) bool {
		return useMatches(use, query, b.Ref.Module, outer, open)
	}
	open := !b.IsFile
	for _, part := range b.Parts {
		if part.Ref == nil {
			continue
		}
		if check(part.Ref.ChunkRef, b.Ref.Labels, open) {
			return true
		}
		outer, inherits := b.Ref.Labels, open
		if len(part.Ref.Labels) > 0 {
			outer, inherits = part.Ref.Labels, false
		}
		for _, f := range part.Ref.Filters {
			if check(f.Ref, outer, inherits) {
				return true
			}
		}
	}
	for _, f := range b.Filters {
		if check(f.Ref, b.Ref.Labels, open) {
			return true
		}
	}
	return false
}

func Depend(blocks []Block, query ChunkRef) string {
	var got []Block
	for _, b := range blocks {
		if blockDepends(b, query) {
			got = append(got, b)
		}
	}
	return FormatBlocks(got)
}

func Refer(blocks []Block, item Item) string {
	var got []Block
	for _, b := range blocks {
		for _, it := range b.Items {
			if sameItem(it, item) {
				got = append(got, b)
				break
			}
		}
	}
	return FormatBlocks(got)
}

func nodeMeta(world *World, node string, chain []Block) (metaInfo, error) {
	maps, err := world.Mappings(node)
	if err != nil {
		return metaInfo{}, err
	}
	return describe(chain, maps)
}

func moduleSeen(seen []ModulePath, m ModulePath) bool {
	for _, s := range seen {
		if modulesEqual(s, m) {
			return true
		}
	}
	return false
}

func Modules(blocks []Block, parent ModulePath, meta metaInfo) string {
	var seen []ModulePath
	for _, b := range blocks {
		mod := b.Ref.Module
		for i := 1; i <= len(mod); i++ {
			pre := mod[:i]
			if moduleSeen(seen, pre) {
				continue
			}
			seen = append(seen, copyModule(pre))
		}
	}
	var rows []namedLine
	for _, m := range seen {
		if _, reserved := reservedNamespace(m); reserved {
			continue
		}
		if len(m) <= len(parent) || !modulesEqual(m[:len(parent)], parent) {
			continue
		}
		line := formatModule(m)
		rows = append(rows, namedLine{line, meta.chunks[line]})
	}
	return formatNamed(rows)
}

func Chunks(blocks []Block, mod ModulePath, meta metaInfo) string {
	seen := map[Name]bool{}
	var rows []namedLine
	for _, b := range blocks {
		if _, reserved := reservedNamespace(b.Ref.Module); reserved {
			continue
		}
		if b.Ref.Chunk == "" || !modulesEqual(b.Ref.Module, mod) {
			continue
		}
		if seen[b.Ref.Chunk] {
			continue
		}
		seen[b.Ref.Chunk] = true
		name := string(b.Ref.Chunk)
		rows = append(rows, namedLine{name, meta.chunks[pathKey(joinPath(mod, name))]})
	}
	return formatNamed(rows)
}

func formatNamed(rows []namedLine) string {
	const tabWidth = 8
	col := 0
	width := 0
	for _, row := range rows {
		if row.desc == "" {
			continue
		}
		if n := utf8.RuneCountInString(row.name) + 1; n > width {
			width = n
		}
	}
	if width > 0 {
		col = (width/tabWidth + 1) * tabWidth
	}
	lines := make([]string, len(rows))
	for i, row := range rows {
		if row.desc == "" {
			lines[i] = row.name
			continue
		}
		lines[i] = row.name + ":" + tabPad(utf8.RuneCountInString(row.name)+1, col, tabWidth) + row.desc
	}
	return joinLines(lines)
}

func tabPad(from, col, tabWidth int) string {
	var b strings.Builder
	pos := from
	for pos < col {
		b.WriteByte('\t')
		pos += tabWidth - pos%tabWidth
	}
	return b.String()
}

func formatUnder(domain []Name, name Name) string {
	if len(domain) == 0 {
		return string(name)
	}
	return formatModule(domain) + "@/" + string(name)
}

func Subdomains(blocks []Block, domain []Name, meta metaInfo) string {
	seen := map[string]bool{}
	var rows []namedLine
	for _, b := range blocks {
		for _, it := range b.Items {
			if len(it) <= len(domain) || !namePrefix(it, domain) {
				continue
			}
			line := formatUnder(domain, it[len(domain)])
			if seen[line] {
				continue
			}
			seen[line] = true
			rows = append(rows, namedLine{line, meta.items[line]})
		}
	}
	return formatNamed(rows)
}

func Items(blocks []Block, domain []Name, meta metaInfo) string {
	seen := map[string]bool{}
	var rows []namedLine
	for _, b := range blocks {
		for _, it := range b.Items {
			if len(it) != len(domain)+1 || !namePrefix(it, domain) {
				continue
			}
			line := formatItem(it)
			if seen[line] {
				continue
			}
			seen[line] = true
			rows = append(rows, namedLine{line, meta.items[pathKey(it)]})
		}
	}
	return formatNamed(rows)
}

func parseUnit(s string) (unit, error) {
	bad := fmt.Errorf("invalid select entry '%s'", s)
	if rest, ok := strings.CutPrefix(s, "Index@/"); ok {
		it, err := ParseItem(rest)
		if err != nil {
			return unit{}, bad
		}
		return unit{item: it}, nil
	}
	if rest, ok := strings.CutPrefix(s, "File@/"); ok {
		names, err := ParseItem(rest)
		if err != nil {
			return unit{}, bad
		}
		parts := make([]string, len(names))
		for i, n := range names {
			parts[i] = string(n)
		}
		return unit{file: strings.Join(parts, "/")}, nil
	}
	ref, err := ParseChunkRef(s)
	if err != nil {
		return unit{}, bad
	}
	return unit{chunk: &ref}, nil
}

func parseUnits(entries []string) ([]unit, error) {
	units := make([]unit, len(entries))
	for i, s := range entries {
		u, err := parseUnit(s)
		if err != nil {
			return nil, err
		}
		units[i] = u
	}
	return units, nil
}

func selection(chain []Block, units []unit) []bool {
	built := constructionBlocks(chain, units)
	in := append([]bool(nil), built...)
	markItems(chain, itemsOf(chain, built, false), true, in)
	reached := append([]bool(nil), in...)
	markContainers(chain, reached, in)
	markItems(chain, itemsOf(chain, built, true), false, in)
	return in
}

func constructionBlocks(chain []Block, units []unit) []bool {
	in := make([]bool, len(chain))
	seen := map[string]bool{}
	var visit func(i int, applied ChunkRef)
	walk := func(ref ChunkRef) {
		for _, i := range effectiveIndexes(chain, ref) {
			visit(i, ref)
		}
	}
	visit = func(i int, applied ChunkRef) {
		key := fmt.Sprintf("%d %s", i, labelsText(applied.Labels))
		if seen[key] {
			return
		}
		seen[key] = true
		in[i] = true
		b := chain[i]
		for _, part := range b.Parts {
			if part.Ref == nil {
				continue
			}
			inner := resolveBody(part.Ref.ChunkRef, applied.Labels, b.Ref.Module)
			walk(inner)
			for _, f := range part.Ref.Filters {
				walk(resolveFilter(f.Ref, inner.Labels))
			}
		}
		for _, f := range b.Filters {
			walk(resolveFilter(f.Ref, applied.Labels))
		}
	}
	for _, u := range units {
		switch {
		case u.chunk != nil:
			walk(*u.chunk)
		case u.file != "":
			walk(ChunkRef{Chunk: Name(u.file)})
		default:
			for i, b := range chain {
				if carries(b.Defines, u.item) {
					visit(i, b.Ref)
				}
			}
		}
	}
	return in
}

func effectiveIndexes(blocks []Block, ref ChunkRef) []int {
	var eff []int
	for i, b := range blocks {
		if !matchRef(b.Ref, ref) {
			continue
		}
		if b.Op == OpSet {
			eff = eff[:0]
		}
		eff = append(eff, i)
	}
	return eff
}

func referred(b Block) []Item {
	var out []Item
	for _, it := range b.Items {
		if !carries(b.Defines, it) {
			out = append(out, it)
		}
	}
	return out
}

func carries(items []Item, it Item) bool {
	for _, x := range items {
		if sameItem(x, it) {
			return true
		}
	}
	return false
}

func itemsOf(chain []Block, marked []bool, defined bool) map[string]bool {
	keys := map[string]bool{}
	for i, b := range chain {
		if marked[i] {
			for _, it := range itemsBy(b, defined) {
				keys[formatItem(it)] = true
			}
		}
	}
	return keys
}

func markItems(chain []Block, keys map[string]bool, defined bool, in []bool) {
	for i, b := range chain {
		for _, it := range itemsBy(b, defined) {
			if keys[formatItem(it)] {
				in[i] = true
				break
			}
		}
	}
}

func itemsBy(b Block, defined bool) []Item {
	if defined {
		return b.Defines
	}
	return referred(b)
}

func markContainers(chain []Block, from []bool, in []bool) {
	byChunk := map[Name][]ChunkRef{}
	for i, b := range chain {
		if from[i] {
			byChunk[b.Ref.Chunk] = append(byChunk[b.Ref.Chunk], b.Ref)
		}
	}
	for i, b := range chain {
		for _, part := range b.Parts {
			if part.Ref != nil && usesAny(b, part.Ref.ChunkRef, byChunk[part.Ref.Chunk]) {
				in[i] = true
				break
			}
		}
	}
}

func usesAny(b Block, use ChunkRef, applied []ChunkRef) bool {
	for _, ref := range applied {
		if useMatches(use, ref, b.Ref.Module, b.Ref.Labels, !b.IsFile) {
			return true
		}
	}
	return false
}
