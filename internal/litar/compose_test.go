package litar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestComposerChain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "base.la"), "@<msg@=base@\n@<extra@+B@\n")
	writeFile(t, filepath.Join(dir, "child.la"), "@<msg@+child@\n")
	writeFile(t, filepath.Join(dir, "origin.txt"), "mapped\n")
	comp := filepath.Join(dir, "composer.json")
	writeFile(t, comp, `[
    {
        "name": "root",
        "archive": "./base.la",
        "description": "the root",
        "mappings": [
            {"path": "data.txt", "fileset": ["data"], "origin": "./origin.txt"}
        ],
        "base": null
    },
    {
        "name": "child",
        "archive": "./child.la",
        "description": "the child",
        "mappings": [],
        "base": "root"
    }
]
`)
	w, err := LoadComposer(comp)
	if err != nil {
		t.Fatal(err)
	}
	if w.Nodes[1].Base != "root" || w.Nodes[0].Description != "the root" {
		t.Fatalf("%+v", w.Nodes)
	}
	blocks, err := w.Chain("child")
	if err != nil {
		t.Fatal(err)
	}
	ev := NewEvaluator(blocks, nil, nil)
	got, err := ev.Eval(mustRef(t, "msg"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "basechild" {
		t.Fatalf("%q", got)
	}
	maps, err := w.Mappings("child")
	if err != nil || len(maps) != 1 || maps[0].Path != "data.txt" {
		t.Fatalf("%v %+v", err, maps)
	}
	if _, err := os.Stat(maps[0].Origin); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Chain("missing"); err == nil || !strings.Contains(err.Error(), "unknown node 'missing'") {
		t.Fatal(err)
	}
}

func TestComposerErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.la"), "@<m@=a@")
	cases := []struct {
		name string
		json string
		msg  string
	}{
		{"empty", `[]`, "composer has no root"},
		{"two", `[
            {"name":"a","archive":"./a.la","description":"","mappings":[],"base":null},
            {"name":"b","archive":"./a.la","description":"","mappings":[],"base":null}
        ]`, "more than one root"},
		{"dup", `[
            {"name":"a","archive":"./a.la","description":"","mappings":[],"base":null},
            {"name":"a","archive":"./b.la","description":"","mappings":[],"base":"a"}
        ]`, "duplicate node 'a'"},
		{"base", `[
            {"name":"a","archive":"./a.la","description":"","mappings":[],"base":"nope"}
        ]`, "base 'nope' of node 'a' is not defined"},
		{"cycle", `[
            {"name":"root","archive":"./a.la","description":"","mappings":[],"base":null},
            {"name":"a","archive":"./a.la","description":"","mappings":[],"base":"b"},
            {"name":"b","archive":"./a.la","description":"","mappings":[],"base":"a"}
        ]`, "cycle in node bases: a -> b -> a"},
		{"missing", `[
            {"name":"root","archive":"no-such.la","description":"","mappings":[],"base":null}
        ]`, "could not find archive 'no-such.la'"},
		{"share", `[
            {"name":"root","archive":"./a.la","description":"","mappings":[],"base":null},
            {"name":"child","archive":"./a.la","description":"","mappings":[],"base":"root"}
        ]`, "share archive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".json")
			writeFile(t, path, tc.json)
			_, err := LoadComposer(path)
			if err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Fatal(err)
			}
		})
	}
}

func TestIncludeSearch(t *testing.T) {
	dir := t.TempDir()
	inc := filepath.Join(dir, "inc")
	if err := os.Mkdir(inc, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(inc, "lib.la"), "@<m@=from-inc@")
	comp := filepath.Join(dir, "c.json")
	writeFile(t, comp, `[{"name":"root","archive":"lib.la","description":"","mappings":[],"base":null}]`)
	t.Setenv("LITAR_INCLUDE", ":"+inc)
	w, err := LoadComposer(comp)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := w.Chain("")
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewEvaluator(blocks, nil, nil).Eval(mustRef(t, "m"), nil)
	if err != nil || got != "from-inc" {
		t.Fatal(err, got)
	}
}

func TestDumpAndDelete(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "a.la")
	writeFile(t, arch, "@<chunk@=A@\n@<chunk@=B@\n@<spec@:chunk@=C@\n")
	comp := filepath.Join(dir, "c.json")
	writeFile(t, comp, `[{"name":"root","archive":"./a.la","description":"d","mappings":[],"base":null}]`)
	w, err := LoadComposer(comp)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.DumpArchives(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(arch)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "@<chunk@=A@") || !strings.Contains(text, "@<chunk@=B@") || !strings.Contains(text, "@<spec@:chunk@=C@") {
		t.Fatalf("dump %s", text)
	}
	if err := w.DeleteChunk("root", mustRef(t, "chunk")); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(arch)
	if strings.Contains(string(data), "@<chunk@=B@") || !strings.Contains(string(data), "spec@:chunk") {
		t.Fatalf("delete chunk %s", data)
	}
	writeFile(t, arch, "@[hello.c@=hi@\n@<hello.c@+more@\n")
	w, err = LoadComposer(comp)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.DeleteFile("root", "hello.c"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(arch)
	if strings.TrimSpace(string(data)) != "" {
		t.Fatalf("delete file %q", data)
	}
	if err := w.DeleteNode("root"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(comp)
	if strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("composer %s", raw)
	}
}
