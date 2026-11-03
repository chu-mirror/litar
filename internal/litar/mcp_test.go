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
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotTools := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		gotTools = append(gotTools, tool.Name)
	}
	if !sameStrings(gotTools, wantTools) {
		t.Fatalf("tools %+v", tools.Tools)
	}
	assertMCPLists(t, ctx, session, wantResources, wantTemplates)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "import_composition",
		Arguments: map[string]any{"path": filepath.Join(dir, "missing.json")},
	})
	if err != nil || res == nil || !res.IsError {
		t.Fatalf("missing import %v %+v", err, res)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_composition",
		Arguments: map[string]any{"path": archDir, "ground": nil},
	})
	if err != nil || res.IsError {
		t.Fatalf("create archive %v %+v", err, res)
	}
	if res.Content[0].(*mcp.TextContent).Text != "0" {
		t.Fatalf("composition id %q", res.Content[0].(*mcp.TextContent).Text)
	}
	compDir, err := identityPath(archDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(comp); err != nil {
		t.Fatal(err)
	}
	assertMCPLists(t, ctx, session, wantResources, wantTemplates)

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_node",
		Arguments: map[string]any{
			"composition": 0,
			"name":        "root",
			"archive":     "a.la",
			"description": "root node",
			"base":        nil,
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("create %v %+v", err, res)
	}
	if res.Content[0].(*mcp.TextContent).Text != "0" {
		t.Fatalf("node id %q", res.Content[0].(*mcp.TextContent).Text)
	}
	writeFile(t, arch, "@<msg@=hello@\n@<msg@=replaced@\n")
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "import_composition",
		Arguments: map[string]any{"path": archDir},
	})
	if err != nil || res.IsError {
		t.Fatalf("reload %v %+v", err, res)
	}
	assertMCPLists(t, ctx, session, wantResources, wantTemplates)

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "evaluate_expression",
		Arguments: map[string]any{
			"composition": 0,
			"node":        0,
			"expression":  "msg",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("eval %v %+v", err, res)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; text != "replaced" {
		t.Fatalf("eval text %q", text)
	}

	got, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "list://composition"})
	if err != nil {
		t.Fatal(err)
	}
	var listed []listJSON
	if err := json.Unmarshal([]byte(got.Contents[0].Text), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Name != compDir || listed[0].Description != "" || listed[0].ID != 0 {
		t.Fatalf("compositions %+v", listed)
	}
	again, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "list://composition"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Contents[0].Text != got.Contents[0].Text {
		t.Fatalf("id changed %s", again.Contents[0].Text)
	}
	tree, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "tree://node/0"})
	if err != nil {
		t.Fatal(err)
	}
	treeText := strings.ReplaceAll(tree.Contents[0].Text, " ", "")
	if !strings.Contains(treeText, `"name":"root"`) || !strings.Contains(treeText, `"id":0`) || !strings.Contains(treeText, `"sub-trees":[]`) {
		t.Fatalf("tree %s", tree.Contents[0].Text)
	}
	blocks, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "blocks://node/0/0"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(blocks.Contents[0].Text, "@<msg@=hello@") {
		t.Fatalf("blocks %s", blocks.Contents[0].Text)
	}
	applied, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "blocks://apply/0/0/1/0"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(applied.Contents[0].Text, "@<msg@=hello@") {
		t.Fatalf("applied %s", applied.Contents[0].Text)
	}
	chunk, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "tree://chunk/0/0"})
	if err != nil {
		t.Fatal(err)
	}
	chunkText := strings.ReplaceAll(chunk.Contents[0].Text, " ", "")
	if !strings.Contains(chunkText, `"name":"msg"`) || !strings.Contains(chunkText, `"id":1`) {
		t.Fatalf("chunk tree %s", chunk.Contents[0].Text)
	}
	files, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "tree://file/0/0"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(files.Contents[0].Text, `"sub-trees":[]`) {
		t.Fatalf("file tree %s", files.Contents[0].Text)
	}
	specs, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "tree://specialization/0/0/1"})
	if err != nil {
		t.Fatal(err)
	}
	var specTree treeJSON
	if err := json.Unmarshal([]byte(specs.Contents[0].Text), &specTree); err != nil {
		t.Fatal(err)
	}
	if specTree.Name != "/" || specTree.ID != 0 || len(specTree.SubTrees) != 0 {
		t.Fatalf("specializations %+v", specTree)
	}
	if _, err := os.Stat(arch); err != nil {
		t.Fatal(err)
	}
	_, err = session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "file://0/0/99"})
	if err == nil || !strings.Contains(err.Error(), "unknown file 99") {
		t.Fatalf("missing file: %v", err)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "apply_block",
		Arguments: map[string]any{
			"composition": 0,
			"node":        0,
			"text":        "@<msg@+!@\n",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("apply %v %+v", err, res)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "evaluate_expression",
		Arguments: map[string]any{
			"composition": 0,
			"node":        0,
			"expression":  "msg",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("eval2 %v %+v", err, res)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; text != "replaced!" {
		t.Fatalf("after apply %q", text)
	}

	data, err := os.ReadFile(arch)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "@<msg@=hello@") || !strings.Contains(string(data), "@<msg@=replaced@") || !strings.Contains(string(data), "@<msg@+!@") {
		t.Fatalf("archive %s", data)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "delete_chunk",
		Arguments: map[string]any{
			"composition": 0,
			"node":        0,
			"chunk":       1,
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("delete chunk %v %+v", err, res)
	}
	data, _ = os.ReadFile(arch)
	if strings.Contains(string(data), "msg") {
		t.Fatalf("still has msg %s", data)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "delete_node",
		Arguments: map[string]any{
			"composition": 0,
			"node":        0,
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("delete node %v %+v", err, res)
	}
	raw, _ := os.ReadFile(comp)
	if strings.TrimSpace(string(raw)) != strings.TrimSpace(string(emptyComposerBytes())) {
		t.Fatalf("composer %s", raw)
	}
	if _, err := os.Stat(arch); !os.IsNotExist(err) {
		t.Fatal("archive still present", err)
	}
	assertMCPLists(t, ctx, session, wantResources, wantTemplates)
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
			defer session.Close()
			res, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "import_composition",
				Arguments: map[string]any{"path": comp},
			})
			if err != nil || res.IsError {
				t.Fatalf("import %v %+v", err, res)
			}
			if res.Content[0].(*mcp.TextContent).Text != "0" {
				t.Fatalf("composition id %q", res.Content[0].(*mcp.TextContent).Text)
			}
			tools, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			var schema map[string]any
			for _, tool := range tools.Tools {
				rejectTypeArrays(t, tool.InputSchema, tool.Name)
				if tool.Name == "create_node" {
					var ok bool
					schema, ok = tool.InputSchema.(map[string]any)
					if !ok {
						t.Fatalf("schema %T", tool.InputSchema)
					}
				}
			}
			if schema == nil {
				t.Fatal("create_node missing")
			}
			var required bool
			for _, req := range schema["required"].([]any) {
				if req == "base" {
					required = true
				}
			}
			if !required {
				t.Fatal("base is not required")
			}
			props := schema["properties"].(map[string]any)
			base := props["base"].(map[string]any)
			if _, ok := base["type"]; ok {
				t.Fatalf("base type %v", base["type"])
			}
			anyOf, ok := base["anyOf"].([]any)
			if !ok || len(anyOf) != 2 {
				t.Fatalf("anyOf %#v", base["anyOf"])
			}
			gotTypes := map[string]bool{}
			for _, branch := range anyOf {
				b := branch.(map[string]any)
				typ, ok := b["type"].(string)
				if !ok {
					t.Fatalf("branch %#v", branch)
				}
				gotTypes[typ] = true
			}
			if !gotTypes["null"] || !gotTypes["integer"] {
				t.Fatalf("branch types %v", gotTypes)
			}
			args := map[string]any{
				"composition": 0,
				"name":        "root",
				"archive":     "./a.la",
				"description": "root node",
			}
			if tc.set {
				args["base"] = tc.base
			}
			res, err = session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "create_node",
				Arguments: args,
			})
			if tc.fail {
				if err != nil || res == nil || !res.IsError {
					t.Fatalf("omit should fail %v %+v", err, res)
				}
				raw, _ := os.ReadFile(comp)
				if strings.TrimSpace(string(raw)) != strings.TrimSpace(string(emptyComposerBytes())) {
					t.Fatalf("composer %s", raw)
				}
				return
			}
			if err != nil || res.IsError {
				t.Fatalf("create %v %+v", err, res)
			}
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
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	for _, tool := range tools.Tools {
		rejectTypeArrays(t, tool.InputSchema, tool.Name)
		if tool.Name != "create_composition" {
			continue
		}
		var ok bool
		schema, ok = tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("schema %T", tool.InputSchema)
		}
	}
	if schema == nil {
		t.Fatal("create_composition missing")
	}
	var required bool
	for _, req := range schema["required"].([]any) {
		if req == "ground" {
			required = true
		}
	}
	if !required {
		t.Fatal("ground is not required")
	}
	props := schema["properties"].(map[string]any)
	groundProp := props["ground"].(map[string]any)
	if _, ok := groundProp["type"]; ok {
		t.Fatalf("ground type %v", groundProp["type"])
	}
	anyOf, ok := groundProp["anyOf"].([]any)
	if !ok || len(anyOf) != 2 {
		t.Fatalf("anyOf %#v", groundProp["anyOf"])
	}
	gotTypes := map[string]bool{}
	for _, branch := range anyOf {
		b := branch.(map[string]any)
		typ, ok := b["type"].(string)
		if !ok {
			t.Fatalf("branch %#v", branch)
		}
		gotTypes[typ] = true
	}
	if !gotTypes["null"] || !gotTypes["string"] {
		t.Fatalf("branch types %v", gotTypes)
	}

	call := func(path string, ground any, set bool) *mcp.CallToolResult {
		t.Helper()
		args := map[string]any{"path": path}
		if set {
			args["ground"] = ground
		}
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "create_composition",
			Arguments: args,
		})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	omit := filepath.Join(dir, "omit")
	if res := call(omit, nil, false); !res.IsError {
		t.Fatalf("omit %+v", res)
	}
	if _, statErr := os.Stat(omit); !os.IsNotExist(statErr) {
		t.Fatal("omit created a directory", statErr)
	}

	nullDir := filepath.Join(dir, "null")
	if res := call(nullDir, nil, true); res.IsError || res.Content[0].(*mcp.TextContent).Text != "0" {
		t.Fatalf("null %+v", res)
	}
	raw, err := os.ReadFile(filepath.Join(nullDir, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(emptyComposerBytes()) {
		t.Fatalf("null ground %s", raw)
	}

	emptyDir := filepath.Join(dir, "empty")
	if res := call(emptyDir, "", true); res.IsError || res.Content[0].(*mcp.TextContent).Text != "1" {
		t.Fatalf("empty %+v", res)
	}
	raw, err = os.ReadFile(filepath.Join(emptyDir, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(emptyComposerBytes()) {
		t.Fatalf("empty ground %s", raw)
	}

	named := filepath.Join(dir, "named")
	res := call(named, "sample-ground", true)
	if res.IsError {
		t.Fatalf("named %+v", res)
	}
	if res.Content[0].(*mcp.TextContent).Text != "2" {
		t.Fatalf("named id %q", res.Content[0].(*mcp.TextContent).Text)
	}
	raw, err = os.ReadFile(filepath.Join(named, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"ground": "sample-ground"`) || strings.Contains(string(raw), `"name"`) {
		t.Fatalf("named %s", raw)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "evaluate_expression",
		Arguments: map[string]any{
			"composition": 2,
			"node":        0,
			"expression":  "msg",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("eval %v %+v", err, res)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; text != "from-ground" {
		t.Fatalf("eval text %q", text)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_node",
		Arguments: map[string]any{
			"composition": 2,
			"name":        "child",
			"archive":     "./child.la",
			"description": "local",
			"base":        0,
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("child %v %+v", err, res)
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
	res = call(missing, "no-such-ground", true)
	if !res.IsError {
		t.Fatalf("missing %+v", res)
	}
	if msg := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(msg, "could not find ground 'no-such-ground'") {
		t.Fatalf("missing text %q", msg)
	}
	if _, statErr := os.Stat(missing); !os.IsNotExist(statErr) {
		t.Fatal("missing ground created a directory", statErr)
	}
}

func sameStrings(a, b []string) bool {
	aa := append([]string{}, a...)
	bb := append([]string{}, b...)
	slices.Sort(aa)
	slices.Sort(bb)
	return slices.Equal(aa, bb)
}
