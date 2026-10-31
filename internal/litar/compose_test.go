package litar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func composerDoc(nodes string) string {
	return "{\n    \"ground\": null,\n    \"nodes\": " + strings.TrimSpace(nodes) + "\n}\n"
}

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
	writeFile(t, comp, composerDoc(`[
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
]`))
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
		{"empty", composerDoc(`[]`), "composer has no root"},
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
			body := tc.json
			if strings.HasPrefix(strings.TrimSpace(body), "[") {
				body = composerDoc(body)
			}
			writeFile(t, path, body)
			_, err := LoadComposer(path)
			if err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Fatal(err)
			}
		})
	}
}

func TestGroundPrecedes(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib")
	common := filepath.Join(lib, "common")
	if err := os.MkdirAll(common, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(common, "base.la"), "@<msg@=base@")
	writeFile(t, filepath.Join(common, "composer.json"), composerDoc(`[
        {"name":"root","archive":"base.la","description":"ground","mappings":[],"base":null}
    ]`))
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "extra.la"), "@<msg@+child@")
	comp := filepath.Join(sub, "composer.json")
	writeFile(t, comp, `{
    "ground": "common",
    "nodes": [
        {"name":"child","archive":"extra.la","description":"local","mappings":[],"base":"root"}
    ]
}
`)
	t.Setenv("LITAR_LIBRARY", lib)
	elsewhere := filepath.Join(dir, "elsewhere")
	if err := os.Mkdir(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(elsewhere)
	w, err := LoadComposer(comp)
	if err != nil {
		t.Fatal(err)
	}
	if w.localAt != 1 || w.Nodes[0].Name != "root" || w.Nodes[1].Name != "child" {
		t.Fatalf("%d %+v", w.localAt, w.Nodes)
	}
	blocks, err := w.Chain("child")
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewEvaluator(blocks, nil, nil).Eval(mustRef(t, "msg"), nil)
	if err != nil || got != "basechild" {
		t.Fatal(err, got)
	}
	if err := w.DeleteNode("root"); err == nil || !strings.Contains(err.Error(), "defined in the ground") {
		t.Fatal(err)
	}
	if err := w.ApplyBlock("root", "@<msg@+x@\n"); err == nil || !strings.Contains(err.Error(), "defined in the ground") {
		t.Fatal(err)
	}
	if err := w.DeleteChunk("root", mustRef(t, "msg")); err == nil || !strings.Contains(err.Error(), "defined in the ground") {
		t.Fatal(err)
	}
	if err := w.DeleteFile("root", "base.la"); err == nil || !strings.Contains(err.Error(), "defined in the ground") {
		t.Fatal(err)
	}
	if err := w.save(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(comp)
	if err != nil {
		t.Fatal(err)
	}
	text := string(saved)
	if strings.Contains(text, `"name": "root"`) || !strings.Contains(text, `"ground": "common"`) || !strings.Contains(text, `"name": "child"`) {
		t.Fatalf("saved %s", text)
	}
}

func TestGroundErrors(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib")
	for _, name := range []string{"a", "b"} {
		g := filepath.Join(lib, name)
		if err := os.MkdirAll(g, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(g, "n.la"), "@<m@=x@")
	}
	writeFile(t, filepath.Join(lib, "a", "composer.json"), `{
    "ground": "b",
    "nodes": [{"name":"A","archive":"n.la","description":"","mappings":[],"base":null}]
}
`)
	writeFile(t, filepath.Join(lib, "b", "composer.json"), `{
    "ground": "a",
    "nodes": [{"name":"B","archive":"n.la","description":"","mappings":[],"base":"A"}]
}
`)
	t.Setenv("LITAR_LIBRARY", lib)
	_, err := LoadComposer(filepath.Join(lib, "a", "composer.json"))
	if err == nil || !strings.Contains(err.Error(), "cycle in grounds") {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "c.json")
	writeFile(t, missing, `{"ground":"nope","nodes":[]}`)
	_, err = LoadComposer(missing)
	if err == nil || !strings.Contains(err.Error(), "could not find ground 'nope'") {
		t.Fatal(err)
	}
}

func TestDumpAndDelete(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "a.la")
	writeFile(t, arch, "@<chunk@=A@\n@<chunk@=B@\n@<spec@:chunk@=C@\n")
	comp := filepath.Join(dir, "c.json")
	writeFile(t, comp, composerDoc(`[{"name":"root","archive":"./a.la","description":"d","mappings":[],"base":null}]`))
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
	if strings.TrimSpace(string(raw)) != strings.TrimSpace(string(emptyComposerBytes())) {
		t.Fatalf("composer %s", raw)
	}
	if _, err := os.Stat(arch); !os.IsNotExist(err) {
		t.Fatal("archive still present", err)
	}
}

func TestDeleteNodeRemovesArchives(t *testing.T) {
	dir := t.TempDir()
	w, err := CreateComposedArchive(dir, "")
	comp := filepath.Join(dir, "composer.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.CreateNode("root", "./root.la", "root", ""); err != nil {
		t.Fatal(err)
	}
	if err := w.CreateNode("child", "./child.la", "child", "root"); err != nil {
		t.Fatal(err)
	}
	if err := w.CreateNode("grand", "./grand.la", "grand", "child"); err != nil {
		t.Fatal(err)
	}
	rootArch := filepath.Join(dir, "root.la")
	childArch := filepath.Join(dir, "child.la")
	grandArch := filepath.Join(dir, "grand.la")
	if err := w.DeleteNode("child"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rootArch); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(childArch); !os.IsNotExist(err) {
		t.Fatal("child archive still present", err)
	}
	if _, err := os.Stat(grandArch); !os.IsNotExist(err) {
		t.Fatal("grand archive still present", err)
	}
	raw, err := os.ReadFile(comp)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "child") || strings.Contains(text, "grand") || !strings.Contains(text, "root") {
		t.Fatalf("composer %s", text)
	}
	if err := os.Remove(rootArch); err != nil {
		t.Fatal(err)
	}
	if err := w.DeleteNode("root"); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(comp)
	if strings.TrimSpace(string(raw)) != strings.TrimSpace(string(emptyComposerBytes())) {
		t.Fatalf("composer %s", raw)
	}
}

func TestCreateMissingArchive(t *testing.T) {
	dir := t.TempDir()
	w, err := CreateComposedArchive(dir, "")
	comp := filepath.Join(dir, "composer.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.CreateNode("root", "./fresh.la", "new", ""); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(dir, "fresh.la")
	st, err := os.Stat(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() != 0 || !st.Mode().IsRegular() {
		t.Fatalf("fresh mode %v size %d", st.Mode(), st.Size())
	}
	loaded, err := LoadComposer(comp)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Nodes) != 1 || loaded.Nodes[0].ArchiveSpec != "./fresh.la" {
		t.Fatalf("%+v", loaded.Nodes)
	}
	if err := w.ApplyBlock("root", "@<msg@=hi@\n"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(fresh)
	if err != nil || !strings.Contains(string(data), "@<msg@=hi@") {
		t.Fatalf("%v %s", err, data)
	}

	old := filepath.Join(dir, "old.la")
	writeFile(t, old, "@<m@=keep@\n")
	if err := w.CreateNode("child", "./old.la", "old", "root"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(old)
	if string(data) != "@<m@=keep@\n" {
		t.Fatalf("truncated %q", data)
	}

	if err := os.Mkdir(filepath.Join(dir, "dir.la"), 0o755); err != nil {
		t.Fatal(err)
	}
	err = w.CreateNode("bad", "./dir.la", "", "root")
	if err == nil || !strings.Contains(err.Error(), "could not find archive './dir.la'") {
		t.Fatal(err)
	}
	err = w.CreateNode("nested", "./no/such.la", "", "root")
	if err == nil || !strings.Contains(err.Error(), "could not write") {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "no")); !os.IsNotExist(statErr) {
		t.Fatal(statErr)
	}

	elsewhere := filepath.Join(dir, "elsewhere")
	if err := os.Mkdir(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(elsewhere)
	if err := w.CreateNode("bare", "bare.la", "bare", "root"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bare.la")); err != nil {
		t.Fatal(err)
	}
	if err := w.ApplyBlock("bare", "@<msg@=bare@\n"); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(dir, "bare.la"))
	if err != nil || string(data) != "@<msg@=bare@\n" {
		t.Fatalf("bare apply %v %q", err, data)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "bare.la")); !os.IsNotExist(err) {
		t.Fatal("archive created in the working directory", err)
	}
	again, err := LoadComposer(comp)
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, n := range again.Nodes {
		if n.Name == "bare" && n.ArchiveSpec == "bare.la" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("%+v", again.Nodes)
	}

	if err := os.Remove(fresh); err != nil {
		t.Fatal(err)
	}
	err = w.CreateNode("dup", "./fresh.la", "", "root")
	if err == nil || !strings.Contains(err.Error(), "share archive") {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(fresh); !os.IsNotExist(statErr) {
		t.Fatal("created archive left behind", statErr)
	}
}

func TestCreateComposedArchiveGround(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib")
	common := filepath.Join(lib, "sample-ground")
	if err := os.MkdirAll(common, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(common, "base.la"), "@<msg@=from-ground@")
	writeFile(t, filepath.Join(common, "composer.json"), composerDoc(`[
        {"name":"root","archive":"base.la","description":"ground","mappings":[],"base":null}
    ]`))
	blank := filepath.Join(lib, "blank-ground")
	if err := os.MkdirAll(blank, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(blank, "composer.json"), string(emptyComposerBytes()))
	t.Setenv("LITAR_LIBRARY", lib)
	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)

	bare := filepath.Join(dir, "bare")
	w, err := CreateComposedArchive(bare, "")
	if err != nil {
		t.Fatal(err)
	}
	if w.Ground != nil || len(w.Nodes) != 0 {
		t.Fatalf("ground %v nodes %+v", w.Ground, w.Nodes)
	}
	raw, err := os.ReadFile(filepath.Join(bare, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(emptyComposerBytes()) {
		t.Fatalf("null ground %s", raw)
	}

	named := filepath.Join(dir, "named")
	w, err = CreateComposedArchive(named, "sample-ground")
	if err != nil {
		t.Fatal(err)
	}
	if w.Ground == nil || *w.Ground != "sample-ground" || w.localAt != 1 || len(w.Nodes) != 1 || w.Nodes[0].Name != "root" {
		t.Fatalf("world ground %v local %d %+v", w.Ground, w.localAt, w.Nodes)
	}
	blocks, err := w.Chain("root")
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewEvaluator(blocks, nil, nil).Eval(mustRef(t, "msg"), nil)
	if err != nil || got != "from-ground" {
		t.Fatal(err, got)
	}
	raw, err = os.ReadFile(filepath.Join(named, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"ground": "sample-ground"`) || strings.Contains(string(raw), `"name"`) {
		t.Fatalf("saved %s", raw)
	}
	if err := w.CreateNode("child", "./child.la", "local", "root"); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(named, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"ground": "sample-ground"`) || !strings.Contains(text, `"name": "child"`) || strings.Contains(text, `"name": "root"`) {
		t.Fatalf("with child %s", text)
	}

	missing := filepath.Join(dir, "missing")
	_, err = CreateComposedArchive(missing, "no-such-ground")
	if err == nil || !strings.Contains(err.Error(), "could not find ground 'no-such-ground'") {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(missing); !os.IsNotExist(statErr) {
		t.Fatal("missing ground created a directory", statErr)
	}

	invalid := filepath.Join(dir, "invalid")
	_, err = CreateComposedArchive(invalid, "a/b")
	if err == nil || !strings.Contains(err.Error(), "invalid ground 'a/b'") {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(invalid); !os.IsNotExist(statErr) {
		t.Fatal("invalid ground created a directory", statErr)
	}

	failed := filepath.Join(dir, "failed")
	_, err = CreateComposedArchive(failed, "blank-ground")
	if err == nil || !strings.Contains(err.Error(), "composer has no root") {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(failed); !os.IsNotExist(statErr) {
		t.Fatal("failed ground left a directory", statErr)
	}

	kept := filepath.Join(dir, "kept")
	if err := os.Mkdir(kept, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = CreateComposedArchive(kept, "blank-ground")
	if err == nil || !strings.Contains(err.Error(), "composer has no root") {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(kept, "composer.json")); !os.IsNotExist(statErr) {
		t.Fatal("composer left behind", statErr)
	}
	if _, statErr := os.Stat(kept); statErr != nil {
		t.Fatal(statErr)
	}
}
