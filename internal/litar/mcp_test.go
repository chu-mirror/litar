package litar

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var wantResources = []string{"path://composer", "tree://node"}

var wantTemplates = []string{
	"blocks://apply/{node}/{chunk_reference}",
	"blocks://depend/{node}/{chunk_reference}",
	"blocks://effective/{node}/{chunk_reference}",
	"blocks://node/{node}",
	"blocks://refer/{node}/{item}",
	"file://{node}/{path}",
	"path://node/{node}",
	"tree://chunk/{node}",
	"tree://file/{node}",
	"tree://item/{node}",
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
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	if len(tools.Tools) != 2 || !names["create_composed_archive"] || !names["specify_composed_archive"] {
		t.Fatalf("tools %+v", tools.Tools)
	}
	assertMCPLists(t, ctx, session, nil, nil)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "specify_composed_archive",
		Arguments: map[string]any{"path": filepath.Join(dir, "missing.json")},
	})
	if err != nil || res == nil || !res.IsError {
		t.Fatalf("missing specify %v %+v", err, res)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_composed_archive",
		Arguments: map[string]any{"path": archDir},
	})
	if err != nil || res.IsError {
		t.Fatalf("create archive %v %+v", err, res)
	}
	writeFile(t, arch, "@<msg@=hello@\n@<msg@=replaced@\n")
	if _, err := os.Stat(comp); err != nil {
		t.Fatal(err)
	}
	assertMCPLists(t, ctx, session, wantResources, wantTemplates)

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_node",
		Arguments: map[string]any{
			"name":        "root",
			"archive":     "./a.la",
			"description": "root node",
			"base":        "",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("create %v %+v", err, res)
	}
	assertMCPLists(t, ctx, session, wantResources, wantTemplates)

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "evaluate_expression",
		Arguments: map[string]any{
			"node":       "root",
			"expression": "msg",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("eval %v %+v", err, res)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; text != "replaced" {
		t.Fatalf("eval text %q", text)
	}

	got, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "path://composer"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Contents[0].Text != archDir {
		t.Fatalf("composer path %q", got.Contents[0].Text)
	}
	tree, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "tree://node"})
	if err != nil {
		t.Fatal(err)
	}
	treeText := strings.ReplaceAll(tree.Contents[0].Text, " ", "")
	if !strings.Contains(treeText, `"name":"root"`) || !strings.Contains(treeText, `"sub-trees":[]`) {
		t.Fatalf("tree %s", tree.Contents[0].Text)
	}
	blocks, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "blocks://node/root"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(blocks.Contents[0].Text, "@<msg@=hello@") {
		t.Fatalf("blocks %s", blocks.Contents[0].Text)
	}
	applied, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "blocks://apply/root/msg"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(applied.Contents[0].Text, "@<msg@=hello@") {
		t.Fatalf("applied %s", applied.Contents[0].Text)
	}
	chunk, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "tree://chunk/root"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(chunk.Contents[0].Text, `"name":"msg"`) {
		t.Fatalf("chunk tree %s", chunk.Contents[0].Text)
	}
	files, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "tree://file/root"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(files.Contents[0].Text, `"sub-trees":[]`) {
		t.Fatalf("file tree %s", files.Contents[0].Text)
	}
	nodePath, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "path://node/root"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(nodePath.Contents[0].Text) != "a.la" {
		t.Fatalf("archive path %q", nodePath.Contents[0].Text)
	}
	_, err = session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "file://root/dir%2Fmissing"})
	if err == nil || !strings.Contains(err.Error(), "file 'dir/missing' is not defined") {
		t.Fatalf("encoded path: %v", err)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "apply_block",
		Arguments: map[string]any{
			"node": "root",
			"text": "@<msg@+!@\n",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("apply %v %+v", err, res)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "evaluate_expression",
		Arguments: map[string]any{
			"node":       "root",
			"expression": "msg",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("eval2 %v %+v", err, res)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; text != "replaced!" {
		t.Fatalf("after apply %q", text)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "dump_archives"})
	if err != nil || res.IsError {
		t.Fatalf("dump %v %+v", err, res)
	}
	data, err := os.ReadFile(arch)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "@<msg@=hello@") || !strings.Contains(string(data), "@<msg@=replaced@") {
		t.Fatalf("dumped %s", data)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "delete_chunk",
		Arguments: map[string]any{
			"node":      "root",
			"reference": "msg",
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
		Name:      "delete_node",
		Arguments: map[string]any{"name": "root"},
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
		{name: "empty", base: "", set: true},
		{name: "null", base: nil, set: true},
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
				Name:      "specify_composed_archive",
				Arguments: map[string]any{"path": comp},
			})
			if err != nil || res.IsError {
				t.Fatalf("specify %v %+v", err, res)
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
			if !gotTypes["null"] || !gotTypes["string"] {
				t.Fatalf("branch types %v", gotTypes)
			}
			args := map[string]any{
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
			if !strings.Contains(string(raw), `"base": null`) || !strings.Contains(string(raw), `"name": "root"`) {
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
	if len(wantRes) == 0 {
		return
	}
	if names["path://composer"] != "composer path" || descs["path://composer"] != "the path to the composer directory" || mimes["path://composer"] != "text/plain" {
		t.Fatalf("composer resource %+v", lr.Resources)
	}
	if names["tree://node"] != "nodes" || mimes["tree://node"] != "application/json" {
		t.Fatalf("node resource %+v", lr.Resources)
	}
}

func sameStrings(a, b []string) bool {
	aa := append([]string{}, a...)
	bb := append([]string{}, b...)
	slices.Sort(aa)
	slices.Sort(bb)
	return slices.Equal(aa, bb)
}
