package litar

import (
	"strings"
	"testing"
)

func mustArch(t *testing.T, src string) []Block {
	t.Helper()
	arch, err := ParseArchive([]byte(src), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	return arch.Blocks
}

func mustEval(t *testing.T, src, expr, want string) {
	t.Helper()
	blocks := mustArch(t, src)
	ref, filters, err := ParsePrint(expr)
	if err != nil {
		t.Fatal(err)
	}
	ev := NewEvaluator(blocks, nil, nil)
	got, err := ev.Eval(ref, filters)
	if err != nil {
		t.Fatalf("eval %s: %v", expr, err)
	}
	if got != want {
		t.Fatalf("eval %s\n got %q\nwant %q", expr, got, want)
	}
}

func TestSuffixAndEffective(t *testing.T) {
	src := "" +
		"@<c@+A@\n" +
		"@<s1@:c@+B@\n" +
		"@<s2@:s1@:c@+C@\n" +
		"@<s2@:c@+D@\n"
	blocks := mustArch(t, src)
	ref, err := ParseChunkRef("s2@:s1@:c")
	if err != nil {
		t.Fatal(err)
	}
	lit, err := Literal(blocks, ref)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(lit, "s2@:c") {
		t.Fatalf("literal included other path\n%s", lit)
	}
	if !strings.Contains(lit, "@<c@+") || !strings.Contains(lit, "@<s1@:c@+") || !strings.Contains(lit, "@<s2@:s1@:c@+") {
		t.Fatalf("literal %s", lit)
	}
	mustEval(t, src, "s2@:s1@:c", "ABC")
	mustEval(t, ""+
		"@<c@=A@\n"+
		"@<s1@:c@=B@\n"+
		"@<s2@:s1@:c@=C@\n",
		"s2@:s1@:c", "C")
	eff, err := Effective(blocks, ref)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(eff, "@<") != 3 {
		t.Fatalf("effective %s", eff)
	}
}

func TestLabelInheritanceAndDot(t *testing.T) {
	src := "" +
		"@<base@+\n" +
		"@<inner@>\n" +
		"@\n" +
		"@<spec@:base@+\n" +
		"!\n" +
		"@\n" +
		"@<inner@=\n" +
		"plain\n" +
		"@\n" +
		"@<spec@:inner@=\n" +
		"special\n" +
		"@\n"
	mustEval(t, src, "spec@:base", "special\n\n!\n")
	mustEval(t, src, "base", "plain\n\n")

	src = "" +
		"@<mod@/chunk@=@<.@/other@>@<..@/rootchunk@>@<sub@/x@>@\n" +
		"@<mod@/other@=O@\n" +
		"@<rootchunk@=R@\n" +
		"@<mod@/sub@/x@=S@\n" +
		"@<sub@/x@=T@\n"
	mustEval(t, src, "mod@/chunk", "ORT")

	_, err := NewEvaluator(mustArch(t, ""+
		"@<top@=\n"+
		"@<..@/top@>\n"+
		"@\n"), nil, nil).Eval(mustRef(t, "top"), nil)
	if err == nil || !strings.Contains(err.Error(), "circular reference top -> top") {
		t.Fatal(err)
	}
}

func mustRef(t *testing.T, s string) ChunkRef {
	t.Helper()
	ref, err := ParseChunkRef(s)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestCycleAndMissing(t *testing.T) {
	src := "" +
		"@<a@=\n" +
		"@<b@>\n" +
		"@\n" +
		"@<b@=\n" +
		"@<a@>\n" +
		"@\n" +
		"@<a@=\n" +
		"ok\n" +
		"@\n"
	ev := NewEvaluator(mustArch(t, src), nil, nil)
	_, err := ev.Eval(mustRef(t, "a"), nil)
	if err == nil || !strings.Contains(err.Error(), "circular reference a -> b -> a") {
		t.Fatal(err)
	}
	mustEval(t, "@<msg@=@<missing@>@", "msg", "")
	ev = NewEvaluator(mustArch(t, "@<msg@=x@"), nil, nil)
	_, err = ev.Eval(mustRef(t, "nope"), nil)
	if err == nil || err.Error() != "chunk 'nope' is not defined" {
		t.Fatal(err)
	}
}

func TestListings(t *testing.T) {
	src := "" +
		"@<a@/b@/c@=\n" +
		"x\n" +
		"@\n" +
		"@<a@/d@=\n" +
		"y\n" +
		"@\n" +
		"@<z@=\n" +
		"q\n" +
		"@~C@/variables@/times@~C@/functions@/main@~only@\n" +
		"@[hello.c@`docs@=\n" +
		"file\n" +
		"@\n"
	blocks := mustArch(t, src)
	meta := newMeta()
	if got := Modules(blocks, nil, meta); got != "a\na@/b\n" {
		t.Fatalf("modules / %q", got)
	}
	parent, _ := ParseModulePath("a")
	if got := Modules(blocks, parent, meta); got != "a@/b\n" {
		t.Fatalf("modules a %q", got)
	}
	if got := Chunks(blocks, nil, meta); got != "z\nhello.c\n" {
		t.Fatalf("chunks / %q", got)
	}
	ab, _ := ParseModulePath("a@/b")
	if got := Chunks(blocks, ab, meta); got != "c\n" {
		t.Fatalf("chunks a/b %q", got)
	}
	if got := Subdomains(blocks, nil, meta); got != "C\nonly\n" {
		t.Fatalf("sub / %q", got)
	}
	dom, _ := ParseModulePath("C")
	if got := Subdomains(blocks, dom, meta); got != "C@/variables\nC@/functions\n" {
		t.Fatalf("sub C %q", got)
	}
	if got := Items(blocks, nil, meta); got != "only\n" {
		t.Fatalf("items / %q", got)
	}
	cv, _ := ParseModulePath("C@/variables")
	if got := Items(blocks, cv, meta); got != "C@/variables@/times\n" {
		t.Fatalf("items %q", got)
	}
	if got := Refer(blocks, mustItem(t, "C@/functions@/main")); !strings.Contains(got, "@<z@=") {
		t.Fatalf("refer %q", got)
	}
	if got := Refer(blocks, mustItem(t, "missing")); got != "" {
		t.Fatalf("refer missing %q", got)
	}
	if got := Modules(blocks, mustMod(t, "nope"), meta); got != "" {
		t.Fatalf("missing module %q", got)
	}
}

func mustItem(t *testing.T, s string) Item {
	t.Helper()
	it, err := ParseItem(s)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func mustMod(t *testing.T, s string) ModulePath {
	t.Helper()
	m, err := ParseModulePath(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDepend(t *testing.T) {
	src := "" +
		"@<chunk 1@=\n" +
		"content\n" +
		"@\n" +
		"\n" +
		"@<chunk 2@=\n" +
		"@<label 1@:chunk 1@>\n" +
		"@\n" +
		"\n" +
		"@<chunk 3@=\n" +
		"content\n" +
		"@|label 2@:chunk 2@\n"
	blocks := mustArch(t, src)
	d1 := Depend(blocks, mustRef(t, "chunk 1"))
	if !strings.Contains(d1, "@<chunk 2@=") || strings.Contains(d1, "@<chunk 3@=") || strings.Contains(d1, "@<chunk 1@=") {
		t.Fatalf("depend chunk 1\n%s", d1)
	}
	d1b := Depend(blocks, mustRef(t, "label 1@:chunk 1"))
	if d1b != d1 {
		t.Fatalf("label depend\n%s\n%s", d1b, d1)
	}
	d2 := Depend(blocks, mustRef(t, "chunk 2"))
	if !strings.Contains(d2, "@<chunk 3@=") || strings.Contains(d2, "@<chunk 2@=") {
		t.Fatalf("depend chunk 2\n%s", d2)
	}
	if Depend(blocks, mustRef(t, "label 2@:chunk 2")) != d2 {
		t.Fatal("label 2")
	}
	if Depend(blocks, mustRef(t, "nope")) != "" {
		t.Fatal("missing depend")
	}
}

func TestLiteralGolden(t *testing.T) {
	src := "" +
		"@<module@/chunk1@=\n" +
		"@<substructure 1@>\n" +
		"@\n" +
		"\n" +
		"@<module@/chunk2@=\n" +
		"@<substructure 2@>\n" +
		"@\n" +
		"\n" +
		"Extend chunk through label.\n" +
		"@<label@:module@/chunk1@=\n" +
		"@<substructure 3@>\n" +
		"@\n"
	want := "" +
		"@<module@/chunk1@=\n" +
		"@<substructure 1@>\n" +
		"@\n" +
		"\n" +
		"Extend chunk through label.\n" +
		"@<label@:module@/chunk1@=\n" +
		"@<substructure 3@>\n" +
		"@\n"
	blocks := mustArch(t, src)
	got, err := Literal(blocks, mustRef(t, "label@:module@/chunk1"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestResolveModule(t *testing.T) {
	base := ModulePath{"mod", "sub"}
	got := resolveModule(base, ModulePath{"..", "..", "x"})
	if formatModule(got) != "x" {
		t.Fatal(formatModule(got))
	}
	got = resolveModule(nil, ModulePath{"..", "x"})
	if formatModule(got) != "x" {
		t.Fatal(formatModule(got))
	}
	got = resolveModule(base, ModulePath{"a", "..", "b"})
	if formatModule(got) != "b" {
		t.Fatal(formatModule(got))
	}
}
