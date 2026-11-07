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
	comp := filepath.Join(dir, "composer.json")
	writeFile(t, filepath.Join(dir, "base.la"), "@<msg@=base@\n@<extra@+B@\n")
	writeFile(t, filepath.Join(dir, "child.la"), "@<msg@+child@\n")
	writeFile(t, filepath.Join(dir, "origin.txt"), "mapped\n")
	writeFile(t, comp, composerDoc(`[
    {
        "name": "root",
        "archive": "./base.la",
        "description": "the root",
        "mappings": [
            {"path": "data.txt", "fileset": ["data"], "origin": "./origin.txt"}
        ],
        "base": []
    },
    {
        "name": "child",
        "archive": "./child.la",
        "description": "the child",
        "mappings": [],
        "base": ["root"]
    }
]`))
	w, err := LoadComposer(comp)
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(w.Nodes[1].Base, []string{"root"}) || w.Nodes[0].Description != "the root" {
		t.Fatalf("%+v", w.Nodes)
	}
	blocks, err := w.Chain("root/child")
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
	maps, err := w.Mappings("root/child")
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

func TestSameNameDifferentParents(t *testing.T) {
	dir := t.TempDir()
	comp := filepath.Join(dir, "composer.json")
	for name, body := range map[string]string{
		"root.la":  "@<x@=\n@\n",
		"left.la":  "@<x@=\n@\n",
		"right.la": "@<x@=\n@\n",
		"leafL.la": "@<msg@=L@\n",
		"leafR.la": "@<msg@=R@\n",
		"again.la": "@<msg@=again@\n",
	} {
		writeFile(t, filepath.Join(dir, name), body)
	}
	writeFile(t, comp, composerDoc(`[
    {"name":"root","archive":"./root.la","description":"","mappings":[],"base":[]},
    {"name":"left","archive":"./left.la","description":"","mappings":[],"base":["root"]},
    {"name":"right","archive":"./right.la","description":"","mappings":[],"base":["root"]},
    {"name":"leaf","archive":"./leafL.la","description":"","mappings":[],"base":["root","left"]},
    {"name":"leaf","archive":"./leafR.la","description":"","mappings":[],"base":["root","right"]},
    {"name":"root","archive":"./again.la","description":"","mappings":[],"base":["root"]}
]`))
	w, err := LoadComposer(comp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Chain("leaf"); err == nil || !strings.Contains(err.Error(), "unknown node 'leaf'") {
		t.Fatal(err)
	}
	left, err := w.Chain("root/left/leaf")
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewEvaluator(left, nil, nil).Eval(mustRef(t, "msg"), nil)
	if err != nil || got != "L" {
		t.Fatalf("%v %q", err, got)
	}
	right, err := w.Chain("root/right/leaf")
	if err != nil {
		t.Fatal(err)
	}
	got, err = NewEvaluator(right, nil, nil).Eval(mustRef(t, "msg"), nil)
	if err != nil || got != "R" {
		t.Fatalf("%v %q", err, got)
	}
	if _, err := w.Chain("left"); err == nil || !strings.Contains(err.Error(), "unknown node 'left'") {
		t.Fatal(err)
	}
	if _, err := w.Chain("root/left"); err != nil {
		t.Fatal(err)
	}
	root, err := w.lookup("root")
	if err != nil || formatNodePath(nodePath(*root)) != "root" {
		t.Fatalf("%v %+v", err, root)
	}
	again, err := w.lookup("root/root")
	if err != nil || again.ArchiveSpec != "./again.la" {
		t.Fatalf("%v %+v", err, again)
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
            {"name":"a","archive":"./a.la","description":"","mappings":[],"base":[]},
            {"name":"b","archive":"./a.la","description":"","mappings":[],"base":[]}
        ]`, "more than one root"},
		{"dup", `[
            {"name":"root","archive":"./a.la","description":"","mappings":[],"base":[]},
            {"name":"a","archive":"./a.la","description":"","mappings":[],"base":["root"]},
            {"name":"a","archive":"./b.la","description":"","mappings":[],"base":["root"]}
        ]`, "duplicate node 'a'"},
		{"base", `[
            {"name":"a","archive":"./a.la","description":"","mappings":[],"base":["nope"]}
        ]`, "base 'nope' of node 'a' is not defined"},
		{"cycle", `[
            {"name":"root","archive":"./a.la","description":"","mappings":[],"base":[]},
            {"name":"a","archive":"./a.la","description":"","mappings":[],"base":["b"]},
            {"name":"b","archive":"./a.la","description":"","mappings":[],"base":["a"]}
        ]`, "base 'b' of node 'a' is not defined"},
		{"nullbase", `[
            {"name":"a","archive":"./a.la","description":"","mappings":[],"base":null}
        ]`, "base must be an array"},
		{"missing", `[
            {"name":"root","archive":"no-such.la","description":"","mappings":[],"base":[]}
        ]`, "could not find archive 'no-such.la'"},
		{"share", `[
            {"name":"root","archive":"./a.la","description":"","mappings":[],"base":[]},
            {"name":"child","archive":"./a.la","description":"","mappings":[],"base":["root"]}
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
        {"name":"root","archive":"base.la","description":"ground","mappings":[],"base":[]}
    ]`))
	sub := filepath.Join(dir, "sub")
	comp := filepath.Join(sub, "composer.json")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "extra.la"), "@<msg@+child@")
	writeFile(t, comp, `{
    "ground": "common",
    "nodes": [
        {"name":"child","archive":"extra.la","description":"local","mappings":[],"base":["root"]}
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
	blocks, err := w.Chain("root/child")
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
    "nodes": [{"name":"A","archive":"n.la","description":"","mappings":[],"base":[]}]
}
`)
	writeFile(t, filepath.Join(lib, "b", "composer.json"), `{
    "ground": "a",
    "nodes": [{"name":"B","archive":"n.la","description":"","mappings":[],"base":["A"]}]
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
	writeFile(t, comp, composerDoc(`[{"name":"root","archive":"./a.la","description":"d","mappings":[],"base":[]}]`))
	w, err := LoadComposer(comp)
	if err != nil {
		t.Fatal(err)
	}
	var data []byte
	if err := w.DumpArchives(); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(arch)
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
	if err := w.CreateNode("root", "./root.la", "root", nil); err != nil {
		t.Fatal(err)
	}
	if err := w.CreateNode("child", "./child.la", "child", []string{"root"}); err != nil {
		t.Fatal(err)
	}
	if err := w.CreateNode("grand", "./grand.la", "grand", []string{"root", "child"}); err != nil {
		t.Fatal(err)
	}
	rootArch := filepath.Join(dir, "root.la")
	var raw []byte
	childArch := filepath.Join(dir, "child.la")
	grandArch := filepath.Join(dir, "grand.la")
	if err := w.DeleteNode("root/child"); err != nil {
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
	raw, err = os.ReadFile(comp)
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
	fresh := filepath.Join(dir, "fresh.la")
	var data []byte
	if err := w.CreateNode("root", "./fresh.la", "new", nil); err != nil {
		t.Fatal(err)
	}
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
	data, err = os.ReadFile(fresh)
	if err != nil || !strings.Contains(string(data), "@<msg@=hi@") {
		t.Fatalf("%v %s", err, data)
	}

	old := filepath.Join(dir, "old.la")
	writeFile(t, old, "@<m@=keep@\n")
	if err := w.CreateNode("child", "./old.la", "old", []string{"root"}); err == nil || !strings.Contains(err.Error(), "archive './old.la' already exists") {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(old)
	if string(data) != "@<m@=keep@\n" {
		t.Fatalf("truncated %q", data)
	}

	if err := os.Mkdir(filepath.Join(dir, "dir.la"), 0o755); err != nil {
		t.Fatal(err)
	}
	err = w.CreateNode("bad", "./dir.la", "", []string{"root"})
	if err == nil || !strings.Contains(err.Error(), "archive './dir.la' already exists") {
		t.Fatal(err)
	}

	err = w.CreateNode("nested", "./no/such.la", "", []string{"root"})
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
	if err := w.CreateNode("bare", "bare.la", "bare", []string{"root"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bare.la")); err != nil {
		t.Fatal(err)
	}
	if err := w.ApplyBlock("root/bare", "@<msg@=bare@\n"); err != nil {
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
	err = w.CreateNode("dup", "./fresh.la", "", []string{"root"})
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
        {"name":"root","archive":"base.la","description":"ground","mappings":[],"base":[]}
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

	var (
		w   *World
		err error
		raw []byte
	)
	bare := filepath.Join(dir, "bare")
	w, err = CreateComposedArchive(bare, "")
	if err != nil {
		t.Fatal(err)
	}
	if w.Ground != nil || len(w.Nodes) != 0 {
		t.Fatalf("ground %v nodes %+v", w.Ground, w.Nodes)
	}
	raw, err = os.ReadFile(filepath.Join(bare, "composer.json"))
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
	if err := w.CreateNode("child", "./child.la", "local", []string{"root"}); err != nil {
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

func orderedRaw(t *testing.T, src string) string {
	t.Helper()
	var b strings.Builder
	for _, bl := range orderOwn(mustArch(t, src)) {
		b.WriteString(bl.Comment)
		b.WriteString(bl.Raw)
	}
	return b.String()
}

func TestOrderOwn(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			"included after includer",
			"@<sub@=x@\n@<top@=\n@<sub@>\n@\n",
			"@<top@=\n@<sub@>\n@\n@<sub@=x@\n",
		},
		{
			"others keep their order",
			"@<a@=1@\n@<sub@=x@\n@<b@=2@\n@<top@=@<sub@>@\n@<c@=3@\n",
			"@<a@=1@\n@<b@=2@\n@<top@=@<sub@>@\n@<sub@=x@\n@<c@=3@\n",
		},
		{
			"labels and relative modules",
			"@<l@:m@/sub@=x@\n@<m@/top@=@<k@:.@/sub@>@\n",
			"@<m@/top@=@<k@:.@/sub@>@\n@<l@:m@/sub@=x@\n",
		},
		{
			"one chunk keeps its order",
			"@<c@=one@\n@<top@=@<c@>@\n@<c@+, two@\n",
			"@<top@=@<c@>@\n@<c@=one@\n@<c@+, two@\n",
		},
		{
			"circular inclusion keeps the order",
			"@<x@=@<y@>@\n@<y@=@<x@>@\n",
			"@<x@=@<y@>@\n@<y@=@<x@>@\n",
		},
		{
			"self inclusion",
			"@<x@=a@\n@<x@+@<l@:x@>@\n",
			"@<x@=a@\n@<x@+@<l@:x@>@\n",
		},
		{
			"comments move with blocks",
			"About sub.\n@<sub@=x@\n\nAbout top.\n@<top@=@<sub@>@\n",
			"\nAbout top.\n@<top@=@<sub@>@\nAbout sub.\n@<sub@=x@\n",
		},
	}
	for _, tc := range cases {
		if got := orderedRaw(t, tc.src); got != tc.want {
			t.Fatalf("%s:\n%q\nwant\n%q", tc.name, got, tc.want)
		}
	}
}

func TestApplyBlockOrders(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "a.la")
	writeFile(t, arch, "@<sub@=x@\n@<c@=one@\n")
	comp := filepath.Join(dir, "composer.json")
	writeFile(t, comp, composerDoc(`[
    {"name":"root","archive":"./a.la","description":"","mappings":[],"base":[]}
]`))
	w, err := LoadComposer(comp)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ApplyBlock("root", "@<top@=@<sub@>@<c@>@"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(arch)
	if err != nil {
		t.Fatal(err)
	}
	want := "@<top@=@<sub@>@<c@>@\n@<sub@=x@\n@<c@=one@\n"
	if string(data) != want {
		t.Fatalf("%q\nwant\n%q", data, want)
	}
	chain, err := w.Chain("root")
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewEvaluator(chain, nil, nil).Eval(mustRef(t, "top"), nil)
	if err != nil || got != "xone" {
		t.Fatalf("%v %q", err, got)
	}
}
