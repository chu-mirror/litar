package litar

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

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

// Literal is the -l listing. A reference with no block is an error.
func Literal(blocks []Block, ref ChunkRef) (string, error) {
	got := matchingBlocks(blocks, ref)
	if len(got) == 0 {
		return "", fmt.Errorf("chunk '%s' is not defined", ref.String())
	}
	return FormatBlocks(got), nil
}

// Effective is the -e listing.
func Effective(blocks []Block, ref ChunkRef) (string, error) {
	got, ok := effectiveBlocks(blocks, ref)
	if !ok {
		return "", fmt.Errorf("chunk '%s' is not defined", ref.String())
	}
	return FormatBlocks(got), nil
}

func useMatches(use ChunkRef, query ChunkRef, header ModulePath) bool {
	mod := use.Module
	if hasRel(mod) {
		mod = resolveModule(header, mod)
	}
	if use.Chunk != query.Chunk || !modulesEqual(mod, query.Module) {
		return false
	}
	return isSuffix(query.Labels, use.Labels)
}

func blockDepends(b Block, query ChunkRef) bool {
	check := func(use ChunkRef) bool {
		return useMatches(use, query, b.Ref.Module)
	}
	for _, part := range b.Parts {
		if part.Ref == nil {
			continue
		}
		if check(part.Ref.ChunkRef) {
			return true
		}
		for _, f := range part.Ref.Filters {
			if check(f.Ref) {
				return true
			}
		}
	}
	for _, f := range b.Filters {
		if check(f.Ref) {
			return true
		}
	}
	return false
}

// Depend is the -d listing. No match is an empty string, not an error.
func Depend(blocks []Block, query ChunkRef) string {
	var got []Block
	for _, b := range blocks {
		if blockDepends(b, query) {
			got = append(got, b)
		}
	}
	return FormatBlocks(got)
}

// Refer is the -r listing.
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

// Modules lists module paths strictly under parent, in first-introduction order.
// A reserved namespace is omitted. A description follows `Name:`, aligned with tabs.
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
		rows = append(rows, namedLine{line, meta.modules[line]})
	}
	return formatNamed(rows)
}

// Chunks lists chunk names whose module equals mod.
// Chunks in a reserved namespace are omitted.
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
		rows = append(rows, namedLine{name, meta.chunks[leafKey(mod, name)]})
	}
	return formatNamed(rows)
}

type namedLine struct {
	name string
	desc string
}

// formatNamed prints one name per line.
// A description is `Name:` then tabs, then the description.
// Tab stops are eight columns apart, and every description in the list
// starts at the same column, past the longest `Name:`.
func formatNamed(rows []namedLine) string {
	const tabWidth = 8
	width := 0
	for _, row := range rows {
		if row.desc == "" {
			continue
		}
		if n := utf8.RuneCountInString(row.name) + 1; n > width {
			width = n
		}
	}
	col := 0
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

// Subdomains lists the name that follows domain, once each.
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
			rows = append(rows, namedLine{line, meta.domains[line]})
		}
	}
	return formatNamed(rows)
}

// Items lists item paths whose prefix before the last name is domain.
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
			rows = append(rows, namedLine{line, meta.items[leafKey(it[:len(it)-1], string(it[len(it)-1]))]})
		}
	}
	return formatNamed(rows)
}
