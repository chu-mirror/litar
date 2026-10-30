package litar

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCP(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "a.la")
	writeFile(t, arch, "@<msg@=hello@\n@<msg@=replaced@\n")
	comp := filepath.Join(dir, "c.json")

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
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "specify_composer" {
		t.Fatalf("tools %+v", tools.Tools)
	}
	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources.Resources) != 0 {
		t.Fatalf("resources %+v", resources.Resources)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "specify_composer",
		Arguments: map[string]any{"path": comp},
	})
	if err != nil || res.IsError {
		t.Fatalf("specify %v %+v", err, res)
	}
	if _, err := os.Stat(comp); err != nil {
		t.Fatal(err)
	}

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
	if got.Contents[0].Text != comp {
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
	if strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("composer %s", raw)
	}
	if _, err := os.Stat(arch); err != nil {
		t.Fatal("archive removed", err)
	}
}
