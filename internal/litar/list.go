package litar

import "fmt"

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

func moduleSeen(seen []ModulePath, m ModulePath) bool {
	for _, s := range seen {
		if modulesEqual(s, m) {
			return true
		}
	}
	return false
}

// Modules lists module paths strictly under parent, in first-introduction order.
func Modules(blocks []Block, parent ModulePath) string {
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
	var lines []string
	for _, m := range seen {
		if len(m) <= len(parent) || !modulesEqual(m[:len(parent)], parent) {
			continue
		}
		lines = append(lines, formatModule(m))
	}
	return joinLines(lines)
}

// Chunks lists chunk names whose module equals mod.
func Chunks(blocks []Block, mod ModulePath) string {
	seen := map[Name]bool{}
	var lines []string
	for _, b := range blocks {
		if b.Ref.Chunk == "" || !modulesEqual(b.Ref.Module, mod) {
			continue
		}
		if seen[b.Ref.Chunk] {
			continue
		}
		seen[b.Ref.Chunk] = true
		lines = append(lines, string(b.Ref.Chunk))
	}
	return joinLines(lines)
}

func formatUnder(domain []Name, name Name) string {
	if len(domain) == 0 {
		return string(name)
	}
	return formatModule(domain) + "@/" + string(name)
}

// Subdomains lists the name that follows domain, once each.
func Subdomains(blocks []Block, domain []Name) string {
	seen := map[string]bool{}
	var lines []string
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
			lines = append(lines, line)
		}
	}
	return joinLines(lines)
}

// Items lists item paths whose prefix before the last name is domain.
func Items(blocks []Block, domain []Name) string {
	seen := map[string]bool{}
	var lines []string
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
			lines = append(lines, line)
		}
	}
	return joinLines(lines)
}
