package litar

import (
	"strings"
	"testing"
)

func TestParseRoundTrip(t *testing.T) {
	src := "" +
		"lead\n" +
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
		"@\n" +
		"tail\n"
	arch, err := ParseArchive([]byte(src), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	if got := arch.Text(); got != src {
		t.Fatalf("round trip\n got %q\nwant %q", got, src)
	}
	if len(arch.Blocks) != 3 {
		t.Fatalf("blocks %d", len(arch.Blocks))
	}
	if arch.Blocks[0].Ref.String() != "module@/chunk1" {
		t.Fatal(arch.Blocks[0].Ref.String())
	}
	if arch.Blocks[2].Ref.String() != "label@:module@/chunk1" {
		t.Fatal(arch.Blocks[2].Ref.String())
	}
	if arch.Trailing != "tail\n" {
		t.Fatalf("trailing %q", arch.Trailing)
	}
	part := arch.Blocks[0].Parts
	if len(part) != 2 || part[0].Ref == nil || part[0].Ref.Chunk != "substructure 1" || part[1].Text != "\n" {
		t.Fatalf("parts %+v", part)
	}
}

func textOf(t *testing.T, b Block) string {
	t.Helper()
	var sb strings.Builder
	for _, p := range b.Parts {
		if p.Ref != nil {
			t.Fatal("expected text")
		}
		sb.WriteString(p.Text)
	}
	return sb.String()
}

func TestLayoutNewline(t *testing.T) {
	cases := []struct {
		src  string
		body string
	}{
		{"@<msg@=\nHello\n@", "Hello\n"},
		{"@<msg@=Hello@", "Hello"},
		{"@<msg@=\r\nHello@", "Hello"},
		{"@<msg@=\rHello@", "Hello"},
		{"@<msg@=\n\rHello@", "\rHello"},
		{"@<msg@=@@<kept@@>@", "@<kept@>"},
	}
	for _, tc := range cases {
		arch, err := ParseArchive([]byte(tc.src), "a.la")
		if err != nil {
			t.Fatalf("%q: %v", tc.src, err)
		}
		if arch.Text() != tc.src {
			t.Fatalf("%q round trip %q", tc.src, arch.Text())
		}
		if got := textOf(t, arch.Blocks[0]); got != tc.body {
			t.Fatalf("%q body %q want %q", tc.src, got, tc.body)
		}
	}
}

func TestNames(t *testing.T) {
	arch, err := ParseArchive([]byte("@< Module 1 @/ chunk @=x@"), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	ref := arch.Blocks[0].Ref
	if len(ref.Module) != 1 || ref.Module[0] != "Module 1" || ref.Chunk != "chunk" {
		t.Fatalf("%+v", ref)
	}
	arch, err = ParseArchive([]byte("@<  @@  @=x@"), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	if arch.Blocks[0].Ref.Chunk != "@" {
		t.Fatalf("chunk %q", arch.Blocks[0].Ref.Chunk)
	}
	_, err = ParseArchive([]byte("@<a@:b@/c@:d@=x@"), "a.la")
	if err == nil || !strings.Contains(err.Error(), "label after module name") {
		t.Fatal(err)
	}
	_, err = ParseArchive([]byte("@<@=x@"), "a.la")
	if err == nil || !strings.Contains(err.Error(), "empty name") {
		t.Fatal(err)
	}
	_, err = ParseArchive([]byte("@<.@/a@=x@"), "a.la")
	if err == nil || !strings.Contains(err.Error(), "relative module name") {
		t.Fatal(err)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		src string
		msg string
	}{
		{"@|nope@", "unexpected control '@|'"},
		{"@~nope@", "unexpected control '@~'"},
		{"@\n", "unexpected control '@"},
		{"@<msg@=Hello", "unterminated block"},
		{"@<msg", "unterminated name"},
		{"@<msg@=\n@<inner", "unterminated name"},
		{"@<msg@=\n@<inner@\n", "unterminated block"},
		{"@<msg@|filt@=\nx@", "unexpected control '@|'"},
		{"@<msg@=\n@=no@", "unexpected control '@='"},
		{"@[path@/no@=x@", "files cannot be modularized"},
		{"@[label@:hello.c@=x@", "files cannot be specialized"},
		{"@[hello.c@+x@", "files are paired only with @="},
	}
	for _, tc := range cases {
		_, err := ParseArchive([]byte(tc.src), "a.la")
		if err == nil || !strings.Contains(err.Error(), tc.msg) {
			t.Fatalf("%q: %v", tc.src, err)
		}
		if !strings.Contains(err.Error(), "in 'a.la'") {
			t.Fatalf("location %v", err)
		}
	}
}

func TestFileItemsPipeline(t *testing.T) {
	src := "" +
		"@[hello.c@`source files@`executables@=\n" +
		"int x;\n" +
		"@|filter@+source files@,hello.c\n" +
		"@~C@/variables@/times@~C@/functions@/main@\n" +
		"@(run.sh@=\n" +
		"echo\n" +
		"@\n" +
		"@<@|list files@+source files@>\n" +
		"@<chunk@=\n" +
		"@<@|list files@+source files@>\n" +
		"@\n"
	// The invocation at top level is not a block. Put the invocation inside.
	src = "" +
		"@[hello.c@`source files@=\n" +
		"int x;\n" +
		"@|upper@+docs@,hello.c\n" +
		"@~C@/variables@/times@~C@/functions@/main@\n" +
		"@(run.sh@=\n" +
		"echo\n" +
		"@\n" +
		"@<chunk@=\n" +
		"@<@|list files@+source files@>\n" +
		"@@~\n" +
		"@\n"
	arch, err := ParseArchive([]byte(src), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	if arch.Text() != src {
		t.Fatalf("round trip\n%q\n%q", arch.Text(), src)
	}
	if len(arch.Blocks) != 3 {
		t.Fatalf("blocks %d", len(arch.Blocks))
	}
	f := arch.Blocks[0]
	if !f.IsFile || f.Executable || f.Path != "hello.c" || len(f.Sets) != 1 || f.Sets[0] != "source files" {
		t.Fatalf("file %+v", f)
	}
	if len(f.Filters) != 1 || f.Filters[0].Ref.Chunk != "upper" || len(f.Filters[0].View) != 2 {
		t.Fatalf("filters %+v", f.Filters)
	}
	if len(f.Items) != 2 || formatItem(f.Items[0]) != "C@/variables@/times" {
		t.Fatalf("items %+v", f.Items)
	}
	if !arch.Blocks[1].Executable {
		t.Fatal("expected executable")
	}
	body := arch.Blocks[2]
	if len(body.Parts) != 2 || body.Parts[0].Ref == nil {
		t.Fatalf("parts %+v", body.Parts)
	}
	if body.Parts[0].Ref.Chunk != "" || len(body.Parts[0].Ref.Filters) != 1 {
		t.Fatalf("invocation %+v", body.Parts[0].Ref)
	}
	if body.Parts[1].Text != "\n@~\n" {
		t.Fatalf("escaped %q", body.Parts[1].Text)
	}
}

func TestCommandNames(t *testing.T) {
	ref, filters, err := ParsePrint("label1@:label2@:module name@/chunk name@|filter1@|filter2")
	if err != nil {
		t.Fatal(err)
	}
	if ref.String() != "label1@:label2@:module name@/chunk name" || len(filters) != 2 {
		t.Fatalf("%s %d", ref, len(filters))
	}
	ref, filters, err = ParsePrint("@|list files@+source files")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Chunk != "" || len(filters) != 1 || filters[0].View[0].Kind != AddSet {
		t.Fatalf("%+v %+v", ref, filters)
	}
	if _, err := ParseChunkRef("a@|b"); err == nil {
		t.Fatal("pipeline on chunk ref")
	}
	if _, _, err := ParsePrint(".@/a"); err == nil || !strings.Contains(err.Error(), "relative module name in expression") {
		t.Fatal(err)
	}
	mod, err := ParseModulePath("/")
	if err != nil || len(mod) != 0 {
		t.Fatal(err, mod)
	}
	mod, err = ParseModulePath("a@/b")
	if err != nil || formatModule(mod) != "a@/b" {
		t.Fatal(err, mod)
	}
	if _, err := ParseModulePath("."); err == nil {
		t.Fatal("dot module")
	}
	item, err := ParseItem("domain@/subdomain@/item")
	if err != nil || formatItem(item) != "domain@/subdomain@/item" {
		t.Fatal(err, item)
	}
}

func TestExplicitRoot(t *testing.T) {
	arch, err := ParseArchive([]byte("@<@/chunk@=z@"), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	if len(arch.Blocks[0].Ref.Module) != 0 || arch.Blocks[0].Ref.Chunk != "chunk" {
		t.Fatalf("%+v", arch.Blocks[0].Ref)
	}
	arch, err = ParseArchive([]byte("@<lab@:@/mod@/chunk@=z@"), "a.la")
	if err != nil {
		t.Fatal(err)
	}
	ref := arch.Blocks[0].Ref
	if ref.String() != "lab@:mod@/chunk" {
		t.Fatal(ref.String())
	}
}
