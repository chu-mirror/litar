package litar

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestReservedMeta(t *testing.T) {
	src := "" +
		"@<note@=\n" +
		"basics@\n" +
		"@<Module@/Basic@=\n" +
		"{\"description\": \"@<note@>\"}\n" +
		"@\n" +
		"@<Chunk@/Basic@/evaluate@=\n" +
		"{\"description\": \"run\"}\n" +
		"@\n" +
		"@<Basic@/evaluate@=\n" +
		"x\n" +
		"@\n" +
		"@<Module@/Text Processing@/Internals@=\n" +
		"{\"description\": \"inside\"}\n" +
		"@\n" +
		"@<Text Processing@/Internals@/helper@=\n" +
		"y\n" +
		"@\n" +
		"@<Domain@/C@/variables@=\n" +
		"{\"description\": \"vars\"}\n" +
		"@\n" +
		"@<Index@/C@/variables@/times@=\n" +
		"{\"description\": \"count\"}\n" +
		"@\n" +
		"@<marked@=\n" +
		"z\n" +
		"@~C@/variables@/times@\n" +
		""
	arch, err := ParseArchive([]byte(src), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := chunkTree(arch.Blocks, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findTree(chunks, "Module"); ok {
		t.Fatalf("reserved module listed: %s", mustTree(t, chunks))
	}
	if _, ok := findTree(chunks, "Chunk"); ok {
		t.Fatalf("reserved chunk namespace listed: %s", mustTree(t, chunks))
	}
	basic, ok := findTree(chunks, "Basic")
	if !ok || basic.Description != "basics" {
		t.Fatalf("Basic %+v", basic)
	}
	ev, ok := findTree(chunks, "Basic", "evaluate")
	if !ok || ev.Description != "run" {
		t.Fatalf("evaluate %+v", ev)
	}
	inner, ok := findTree(chunks, "Text Processing", "Internals")
	if !ok || inner.Description != "inside" {
		t.Fatalf("internals %+v", inner)
	}
	if _, ok := findTree(chunks, "Text Processing", "Internals", "helper"); !ok {
		t.Fatalf("helper missing: %s", mustTree(t, chunks))
	}
	items, err := itemTree(arch.Blocks, nil)
	if err != nil {
		t.Fatal(err)
	}
	vars, ok := findTree(items, "C", "variables")
	if !ok || vars.Description != "vars" {
		t.Fatalf("domain %+v", vars)
	}
	times, ok := findTree(items, "C", "variables", "times")
	if !ok || times.Description != "count" {
		t.Fatalf("item %+v", times)
	}
	bad, err := ParseArchive([]byte("@<Module@/Basic@=\nnot json\n@\n@<Basic@/x@=\n@\n"), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chunkTree(bad.Blocks, nil); err == nil || !strings.Contains(err.Error(), "meta chunk 'Module@/Basic'") {
		t.Fatalf("bad meta: %v", err)
	}
}

func TestSpecializationMeta(t *testing.T) {
	src := "" +
		"@<msg@=\nplain\n@\n" +
		"@<label2@:label1@:msg@=\nbody\n@\n" +
		"@<Specialization@/label1@/label2@=\n{\"description\": \"the spec\"}\n@\n"
	arch, err := ParseArchive([]byte(src), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := chunkTree(arch.Blocks, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findTree(chunks, "Specialization"); ok {
		t.Fatalf("reserved specialization listed: %s", mustTree(t, chunks))
	}
	if _, ok := findTree(chunks, "msg"); !ok {
		t.Fatal("msg missing")
	}
	meta, err := describe(arch.Blocks, nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta.specs["label2@:label1"] != "the spec" {
		t.Fatalf("specs %+v", meta.specs)
	}
	if specName([]Name{"label1", "label2"}) != "label2@:label1" {
		t.Fatal(specName([]Name{"label1", "label2"}))
	}
	chunk := ChunkRef{Chunk: "msg"}
	tree, err := specializationTree(arch.Blocks, nil, chunk)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Name != "/" || tree.ID != 0 || tree.Description != "" || len(tree.SubTrees) != 1 {
		t.Fatalf("root %+v", tree)
	}
	label1 := tree.SubTrees[0]
	if label1.Name != "label1" || label1.ID != 1 || label1.Description != "" || len(label1.SubTrees) != 1 {
		t.Fatalf("label1 %+v", label1)
	}
	label2 := label1.SubTrees[0]
	if label2.Name != "label2" || label2.ID != 2 || label2.Description != "the spec" || len(label2.SubTrees) != 0 {
		t.Fatalf("label2 %+v", label2)
	}
	again, err := specializationTree(arch.Blocks, nil, chunk)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != tree.ID || again.SubTrees[0].ID != label1.ID || again.SubTrees[0].SubTrees[0].ID != label2.ID {
		t.Fatalf("ids %+v", again)
	}
	if got := Modules(arch.Blocks, nil, meta); strings.Contains(got, "Specialization") {
		t.Fatalf("listed %q", got)
	}
	refOnly, err := ParseArchive([]byte("@<msg@=\nplain\n@\n@<other@=\n@<hello@:executable@:msg@>\n@\n"), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	refTree, err := specializationTree(refOnly.Blocks, nil, ChunkRef{Chunk: "msg"})
	if err != nil {
		t.Fatal(err)
	}
	exec, ok := findTree(refTree, "executable")
	if !ok || exec.ID != 1 {
		t.Fatalf("executable %+v", refTree)
	}
	hello, ok := findTree(refTree, "executable", "hello")
	if !ok || hello.ID != 2 || hello.Description != "" {
		t.Fatalf("hello %+v", refTree)
	}
}

func TestListingDescriptions(t *testing.T) {
	src := "" +
		"@<Module@/Basic@=\n" +
		"{\"description\": \"basics\"}\n" +
		"@\n" +
		"@<Chunk@/Basic@/evaluate@=\n" +
		"{\"description\": \"run\"}\n" +
		"@\n" +
		"@<Basic@/evaluate@=\n" +
		"x\n" +
		"@\n" +
		"@<Module@/Text Processing@/Internals@=\n" +
		"{\"description\": \"inside\"}\n" +
		"@\n" +
		"@<Text Processing@/Internals@/helper@=\n" +
		"y\n" +
		"@\n" +
		"@<Domain@/C@/variables@=\n" +
		"{\"description\": \"vars\"}\n" +
		"@\n" +
		"@<Index@/C@/variables@/times@=\n" +
		"{\"description\": \"count\"}\n" +
		"@\n" +
		"@<marked@=\n" +
		"z\n" +
		"@~C@/variables@/times@\n" +
		""
	arch, err := ParseArchive([]byte(src), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := describe(arch.Blocks, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := Modules(arch.Blocks, nil, meta); got != "Basic:\t\t\t\tbasics\nText Processing\nText Processing@/Internals:\tinside\n" {
		t.Fatalf("modules %q", got)
	}
	if got := Chunks(arch.Blocks, mustMod(t, "Module"), meta); got != "" {
		t.Fatalf("reserved chunks %q", got)
	}
	if got := Chunks(arch.Blocks, mustMod(t, "Basic"), meta); got != "evaluate:\trun\n" {
		t.Fatalf("chunks %q", got)
	}
	if got := Subdomains(arch.Blocks, nil, meta); got != "C\n" {
		t.Fatalf("sub / %q", got)
	}
	if got := Subdomains(arch.Blocks, mustMod(t, "C"), meta); got != "C@/variables:\tvars\n" {
		t.Fatalf("sub C %q", got)
	}
	if got := Items(arch.Blocks, mustMod(t, "C@/variables"), meta); got != "C@/variables@/times:\tcount\n" {
		t.Fatalf("items %q", got)
	}
}

func TestLibraryModuleDescriptions(t *testing.T) {
	chain, maps := exampleChain(t, "Basic/C Structure/Light/Test Light")
	tree, err := chunkTree(chain, maps)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := describe(chain, maps)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Basic":           "Some common structures and filters.",
		"Text Processing": "Text processing based on perl.",
		"C Structure":     "Common structures in C programming",
		"Light":           "Light is a framework mainly for C programming",
	}
	if name, found := findNamed(tree, "Module"); found {
		t.Fatalf("tree contains %s", name)
	}
	for name, desc := range want {
		node, ok := findTree(tree, name)
		if !ok || node.Description != desc {
			t.Fatalf("%s %+v", name, node)
		}
	}
	inner, ok := findTree(tree, "Text Processing", "Internals")
	if !ok || inner.Description != "" {
		t.Fatalf("internals %+v", inner)
	}
	listed := Modules(chain, nil, meta)
	if strings.Contains(listed, "\nModule\n") || strings.HasPrefix(listed, "Module\n") || strings.HasPrefix(listed, "Module:") {
		t.Fatalf("reserved module listed %q", listed)
	}
	col := -1
	for name, desc := range want {
		line := listedLine(listed, name)
		gotName, gotDesc, gotCol, ok := splitNamed(line)
		if !ok || gotName != name || gotDesc != desc {
			t.Fatalf("%s line %q", name, line)
		}
		if col == -1 {
			col = gotCol
		} else if gotCol != col {
			t.Fatalf("%s column %d, want %d", name, gotCol, col)
		}
	}
}

func exampleChain(t *testing.T, np string) ([]Block, []Mapping) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	t.Setenv("LITAR_LIBRARY", filepath.Join(root, "grounds"))
	w, err := LoadComposer(filepath.Join(root, "examples", "c", "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	chain, err := w.Chain(np)
	if err != nil {
		t.Fatal(err)
	}
	maps, err := w.Mappings(np)
	if err != nil {
		t.Fatal(err)
	}
	return chain, maps
}

func TestProgramHelloModuleSpecializations(t *testing.T) {
	chain, maps := exampleChain(t, "Basic/C Structure/Program Hello")
	chunk := ChunkRef{Module: ModulePath{"C Structure"}, Chunk: "module"}
	tree, err := specializationTree(chain, maps, chunk)
	if err != nil {
		t.Fatal(err)
	}
	exec, ok := findTree(tree, "executable")
	if !ok {
		t.Fatalf("missing executable: %s", mustTree(t, tree))
	}
	hello, ok := findTree(tree, "executable", "hello")
	if !ok {
		t.Fatalf("missing hello: %s", mustTree(t, tree))
	}
	if exec.ID == hello.ID {
		t.Fatalf("ids %+v", tree)
	}
	labels, err := specLabels(chain, maps, chunk, hello.ID)
	if err != nil {
		t.Fatal(err)
	}
	if labelsText(labels) != "hello@:executable" {
		t.Fatalf("labels %q id %d", labelsText(labels), hello.ID)
	}
}

func listedLine(text, name string) string {
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if line == name || strings.HasPrefix(line, name+":") {
			return line
		}
	}
	return ""
}

func splitNamed(line string) (name, desc string, col int, ok bool) {
	label, rest, found := strings.Cut(line, ":")
	if !found {
		return "", "", 0, false
	}
	pos := utf8.RuneCountInString(label) + 1
	for len(rest) > 0 && rest[0] == '\t' {
		pos += 8 - pos%8
		rest = rest[1:]
	}
	if rest == "" || strings.Contains(rest, "\t") {
		return "", "", 0, false
	}
	return label, rest, pos, true
}

func findTree(t treeJSON, path ...string) (treeJSON, bool) {
	cur := t
	for _, name := range path {
		found := false
		for _, sub := range cur.SubTrees {
			if sub.Name == name {
				cur = sub
				found = true
				break
			}
		}
		if !found {
			return treeJSON{}, false
		}
	}
	return cur, true
}

func findNamed(t treeJSON, name string) (string, bool) {
	if t.Name == name {
		return t.Name, true
	}
	for _, sub := range t.SubTrees {
		if path, ok := findNamed(sub, name); ok {
			if t.Name == "" || t.Name == "/" {
				return path, true
			}
			return t.Name + "/" + path, true
		}
	}
	return "", false
}

func mustTree(t *testing.T, tree treeJSON) string {
	t.Helper()
	raw, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
