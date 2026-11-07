package litar

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var wantResources = []string{"list://composition"}

var wantTemplates = []string{
	"blocks://apply/{composition}/{node}/{chunk}/{specialization}",
	"blocks://depend/{composition}/{node}/{chunk}/{specialization}",
	"blocks://effective/{composition}/{node}/{chunk}/{specialization}",
	"blocks://node/{composition}/{node}",
	"blocks://refer/{composition}/{node}/{item}",
	"file://{composition}/{node}/{path}",
	"tree://specialization/{composition}/{node}/{chunk}",
	"tree://chunk/{composition}/{node}",
	"tree://file/{composition}/{node}",
	"tree://item/{composition}/{node}",
	"tree://node/{composition}",
}

var wantTools = []string{
	"import_composition",
	"create_composition",
	"create_node",
	"delete_node",
	"delete_chunk",
	"delete_file",
	"evaluate_expression",
	"apply_block",
}

func TestMCP(t *testing.T) {
	dir := t.TempDir()
	archDir := filepath.Join(dir, "archive")
	arch := filepath.Join(archDir, "a.la")
	comp := filepath.Join(archDir, "composer.json")
	ctx := context.Background()
	session := connectMCP(t, ctx)
	var gotTools []string
	for _, tool := range listTools(t, ctx, session) {
		gotTools = append(gotTools, tool.Name)
	}
	if !sameStrings(gotTools, wantTools) {
		t.Fatalf("tools %v", gotTools)
	}
	assertMCPLists(t, ctx, session, wantResources, wantTemplates)
	callToolError(t, ctx, session, "import_composition", map[string]any{"path": filepath.Join(dir, "missing.json")})
	if id := callTool(t, ctx, session, "create_composition", map[string]any{"path": archDir, "ground": nil}); id != "0" {
		t.Fatalf("composition id %q", id)
	}
	if _, err := os.Stat(comp); err != nil {
		t.Fatal(err)
	}
	assertMCPLists(t, ctx, session, wantResources, wantTemplates)
	if id := callTool(t, ctx, session, "create_node", map[string]any{
		"composition": 0,
		"name":        "root",
		"archive":     "a.la",
		"description": "root node",
		"base":        nil,
	}); id != "0" {
		t.Fatalf("node id %q", id)
	}
	writeFile(t, arch, "@<msg@=hello@\n@<msg@=replaced@\n")
	callTool(t, ctx, session, "import_composition", map[string]any{"path": archDir})
	assertMCPLists(t, ctx, session, wantResources, wantTemplates)
	if text := callTool(t, ctx, session, "evaluate_expression", map[string]any{
		"composition": 0,
		"node":        0,
		"expression":  "msg",
	}); text != "replaced" {
		t.Fatalf("eval text %q", text)
	}
	compDir, err := identityPath(archDir)
	if err != nil {
		t.Fatal(err)
	}
	listText := readResource(t, ctx, session, "list://composition")
	var listed []listJSON
	if err := json.Unmarshal([]byte(listText), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Name != compDir || listed[0].Description != "" || listed[0].ID != 0 {
		t.Fatalf("compositions %+v", listed)
	}
	if again := readResource(t, ctx, session, "list://composition"); again != listText {
		t.Fatalf("id changed %s", again)
	}
	tree := readResource(t, ctx, session, "tree://node/0")
	treeText := strings.ReplaceAll(tree, " ", "")
	if !strings.Contains(treeText, `"name":"root"`) || !strings.Contains(treeText, `"id":0`) || !strings.Contains(treeText, `"sub-trees":[]`) {
		t.Fatalf("tree %s", tree)
	}
	chunk := readResource(t, ctx, session, "tree://chunk/0/0")
	chunkText := strings.ReplaceAll(chunk, " ", "")
	if !strings.Contains(chunkText, `"name":"msg"`) || !strings.Contains(chunkText, `"id":1`) {
		t.Fatalf("chunk tree %s", chunk)
	}
	if files := readResource(t, ctx, session, "tree://file/0/0"); !strings.Contains(files, `"sub-trees":[]`) {
		t.Fatalf("file tree %s", files)
	}
	if blocks := readResource(t, ctx, session, "blocks://node/0/0"); !strings.Contains(blocks, "@<msg@=hello@") {
		t.Fatalf("blocks %s", blocks)
	}
	if applied := readResource(t, ctx, session, "blocks://apply/0/0/1/0"); !strings.Contains(applied, "@<msg@=hello@") {
		t.Fatalf("applied %s", applied)
	}
	var specTree treeJSON
	if err := json.Unmarshal([]byte(readResource(t, ctx, session, "tree://specialization/0/0/1")), &specTree); err != nil {
		t.Fatal(err)
	}
	if specTree.Name != "/" || specTree.ID != 0 || len(specTree.SubTrees) != 0 {
		t.Fatalf("specializations %+v", specTree)
	}
	if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "file://0/0/99"}); err == nil || !strings.Contains(err.Error(), "unknown file 99") {
		t.Fatalf("missing file: %v", err)
	}
	callTool(t, ctx, session, "apply_block", map[string]any{
		"composition": 0,
		"node":        0,
		"text":        "@<msg@+!@\n",
	})
	if text := callTool(t, ctx, session, "evaluate_expression", map[string]any{
		"composition": 0,
		"node":        0,
		"expression":  "msg",
	}); text != "replaced!" {
		t.Fatalf("after apply %q", text)
	}
	data, _ := os.ReadFile(arch)
	if strings.Contains(string(data), "@<msg@=hello@") || !strings.Contains(string(data), "@<msg@=replaced@") || !strings.Contains(string(data), "@<msg@+!@") {
		t.Fatalf("archive %s", data)
	}
	callTool(t, ctx, session, "delete_chunk", map[string]any{"composition": 0, "node": 0, "chunk": 1})
	if data, _ := os.ReadFile(arch); strings.Contains(string(data), "msg") {
		t.Fatalf("still has msg %s", data)
	}
	callTool(t, ctx, session, "delete_node", map[string]any{"composition": 0, "node": 0})
	raw, _ := os.ReadFile(comp)
	if strings.TrimSpace(string(raw)) != strings.TrimSpace(string(emptyComposerBytes())) {
		t.Fatalf("composer %s", raw)
	}
	if _, err := os.Stat(arch); !os.IsNotExist(err) {
		t.Fatal("archive still present", err)
	}
	assertMCPLists(t, ctx, session, wantResources, wantTemplates)
}

func connectMCP(t *testing.T, ctx context.Context) *mcp.ClientSession {
	t.Helper()
	ag := newAgent()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	serverT, clientT := mcp.NewInMemoryTransports()
	if _, err := ag.server.Connect(ctx, serverT, nil); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func callTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || res.IsError {
		t.Fatalf("%s %v %+v", name, err, res)
	}
	return res.Content[0].(*mcp.TextContent).Text
}

func callToolError(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || !res.IsError {
		t.Fatalf("%s should fail %v %+v", name, err, res)
	}
	return res.Content[0].(*mcp.TextContent).Text
}

func readResource(t *testing.T, ctx context.Context, session *mcp.ClientSession, uri string) string {
	t.Helper()
	got, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatal(err)
	}
	return got.Contents[0].Text
}

func listTools(t *testing.T, ctx context.Context, session *mcp.ClientSession) []*mcp.Tool {
	t.Helper()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	return tools.Tools
}

func TestCreateNodeEmptyBase(t *testing.T) {
	cases := []struct {
		name string
		base any
		set  bool
		fail bool
	}{
		{name: "omit", fail: true},
		{name: "null", base: nil, set: true},
		{name: "array", base: []any{}, set: true, fail: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			comp := filepath.Join(dir, "c.json")
			writeFile(t, comp, string(emptyComposerBytes()))
			ctx := context.Background()
			session := connectMCP(t, ctx)
			if id := callTool(t, ctx, session, "import_composition", map[string]any{"path": comp}); id != "0" {
				t.Fatalf("composition id %q", id)
			}
			checkNullableArg(t, ctx, session, "create_node", "base", "integer")
			args := map[string]any{
				"composition": 0,
				"name":        "root",
				"archive":     "./a.la",
				"description": "root node",
			}
			if tc.set {
				args["base"] = tc.base
			}
			if tc.fail {
				callToolError(t, ctx, session, "create_node", args)
				raw, _ := os.ReadFile(comp)
				if strings.TrimSpace(string(raw)) != strings.TrimSpace(string(emptyComposerBytes())) {
					t.Fatalf("composer %s", raw)
				}
				return
			}
			callTool(t, ctx, session, "create_node", args)
			raw, err := os.ReadFile(comp)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `"base": []`) || !strings.Contains(string(raw), `"name": "root"`) {
				t.Fatalf("composer %s", raw)
			}
			if _, err := os.Stat(filepath.Join(dir, "a.la")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func checkNullableArg(t *testing.T, ctx context.Context, session *mcp.ClientSession, tool, arg, typ string) {
	t.Helper()
	var schema map[string]any
	for _, tl := range listTools(t, ctx, session) {
		rejectTypeArrays(t, tl.InputSchema, tl.Name)
		if tl.Name != tool {
			continue
		}
		var ok bool
		schema, ok = tl.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("schema %T", tl.InputSchema)
		}
	}
	if schema == nil {
		t.Fatalf("%s missing", tool)
	}
	if !slices.Contains(schema["required"].([]any), any(arg)) {
		t.Fatalf("%s is not required", arg)
	}
	prop := schema["properties"].(map[string]any)[arg].(map[string]any)
	if _, ok := prop["type"]; ok {
		t.Fatalf("%s type %v", arg, prop["type"])
	}
	anyOf, ok := prop["anyOf"].([]any)
	if !ok || len(anyOf) != 2 {
		t.Fatalf("anyOf %#v", prop["anyOf"])
	}
	gotTypes := map[string]bool{}
	for _, branch := range anyOf {
		bt, ok := branch.(map[string]any)["type"].(string)
		if !ok {
			t.Fatalf("branch %#v", branch)
		}
		gotTypes[bt] = true
	}
	if !gotTypes["null"] || !gotTypes[typ] {
		t.Fatalf("branch types %v", gotTypes)
	}
}

func rejectTypeArrays(t *testing.T, v any, path string) {
	t.Helper()
	switch x := v.(type) {
	case map[string]any:
		if typ, ok := x["type"].([]any); ok {
			t.Fatalf("%s type is an array: %v", path, typ)
		}
		for k, child := range x {
			rejectTypeArrays(t, child, path+"."+k)
		}
	case []any:
		for i, child := range x {
			rejectTypeArrays(t, child, path+"["+strconv.Itoa(i)+"]")
		}
	}
}

func assertMCPLists(t *testing.T, ctx context.Context, session *mcp.ClientSession, wantRes, wantTmpl []string) {
	t.Helper()
	lr, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var gotRes []string
	names := map[string]string{}
	descs := map[string]string{}
	mimes := map[string]string{}
	for _, r := range lr.Resources {
		gotRes = append(gotRes, r.URI)
		names[r.URI] = r.Name
		descs[r.URI] = r.Description
		mimes[r.URI] = r.MIMEType
	}
	lt, err := session.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var gotTmpl []string
	for _, r := range lt.ResourceTemplates {
		gotTmpl = append(gotTmpl, r.URITemplate)
	}
	if !sameStrings(gotRes, wantRes) {
		t.Fatalf("resources %v", gotRes)
	}
	if !sameStrings(gotTmpl, wantTmpl) {
		t.Fatalf("templates %v", gotTmpl)
	}
	if names["list://composition"] != "compositions" || descs["list://composition"] != "a list of editable compositions" || mimes["list://composition"] != "application/json" {
		t.Fatalf("composition resource %+v", lr.Resources)
	}
}

func TestCreateComposedArchiveGroundTool(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib")
	common := filepath.Join(lib, "sample-ground")
	if err := os.MkdirAll(common, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(common, "base.la"), "@<msg@=from-ground@")
	writeFile(t, filepath.Join(common, "composer.json"), `{
    "description": "",
    "ground": null,
    "nodes": [
        {"name":"root","archive":"base.la","description":"ground","mappings":[],"base":[]}
    ]
}
`)
	t.Setenv("LITAR_LIBRARY", lib)
	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	ctx := context.Background()
	session := connectMCP(t, ctx)
	checkNullableArg(t, ctx, session, "create_composition", "ground", "string")
	omit := filepath.Join(dir, "omit")
	callToolError(t, ctx, session, "create_composition", map[string]any{"path": omit})
	if _, err := os.Stat(omit); !os.IsNotExist(err) {
		t.Fatal("omit created a directory", err)
	}
	for i, tc := range []struct {
		name   string
		ground any
	}{{"null", nil}, {"empty", ""}} {
		path := filepath.Join(dir, tc.name)
		if id := callTool(t, ctx, session, "create_composition", map[string]any{"path": path, "ground": tc.ground}); id != strconv.Itoa(i) {
			t.Fatalf("%s id %q", tc.name, id)
		}
		raw, err := os.ReadFile(filepath.Join(path, "composer.json"))
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != string(emptyComposerBytes()) {
			t.Fatalf("%s ground %s", tc.name, raw)
		}
	}
	named := filepath.Join(dir, "named")
	if id := callTool(t, ctx, session, "create_composition", map[string]any{"path": named, "ground": "sample-ground"}); id != "2" {
		t.Fatalf("named id %q", id)
	}
	raw, err := os.ReadFile(filepath.Join(named, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"ground": "sample-ground"`) || strings.Contains(string(raw), `"name"`) {
		t.Fatalf("named %s", raw)
	}
	if text := callTool(t, ctx, session, "evaluate_expression", map[string]any{
		"composition": 2,
		"node":        0,
		"expression":  "msg",
	}); text != "from-ground" {
		t.Fatalf("eval text %q", text)
	}
	callTool(t, ctx, session, "create_node", map[string]any{
		"composition": 2,
		"name":        "child",
		"archive":     "./child.la",
		"description": "local",
		"base":        0,
	})
	withChild, _ := os.ReadFile(filepath.Join(named, "composer.json"))
	if text := string(withChild); !strings.Contains(text, `"ground": "sample-ground"`) || !strings.Contains(text, `"name": "child"`) || strings.Contains(text, `"name": "root"`) {
		t.Fatalf("with child %s", text)
	}
	missing := filepath.Join(dir, "missing")
	msg := callToolError(t, ctx, session, "create_composition", map[string]any{"path": missing, "ground": "no-such-ground"})
	if !strings.Contains(msg, "could not find ground 'no-such-ground'") {
		t.Fatalf("missing text %q", msg)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("missing ground created a directory", err)
	}
}

func sameStrings(a, b []string) bool {
	aa := append([]string{}, a...)
	bb := append([]string{}, b...)
	slices.Sort(aa)
	slices.Sort(bb)
	return slices.Equal(aa, bb)
}
