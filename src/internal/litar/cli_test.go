package litar

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runOut(t *testing.T, opts Options) (int, string, string) {
	t.Helper()
	var out, err bytes.Buffer
	code := run(opts, &out, &err)
	return code, out.String(), err.String()
}

func TestCLIErrors(t *testing.T) {
	cases := []struct {
		opts Options
		msg  string
	}{
		{Options{}, missingAction},
		{Options{File: "a.la"}, missingAction},
		{Options{Print: "a", Literal: "a"}, "cannot combine print and literal"},
		{Options{Reference: "a", Agent: true}, "cannot combine reference and agent"},
		{Options{Print: "msg"}, "missing -f, --file"},
	}
	for _, tc := range cases {
		code, out, err := runOut(t, tc.opts)
		if code == 0 || out != "" || !strings.Contains(err, tc.msg) || !strings.HasPrefix(err, "Error: ") {
			t.Fatalf("%+v code %d out %q err %q", tc.opts, code, out, err)
		}
	}
}

func TestCLIPrint(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.la"), "@<msg@=\nHello\n@\n")
	writeFile(t, filepath.Join(dir, "composer.json"), composerDoc(`[
    {"name":"root","archive":"./a.la","description":"","mappings":[],"base":[]}
]`))
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	var (
		code    int
		out     string
		errText string
	)
	code, out, errText = runOut(t, Options{File: "a.la", Print: "msg"})
	if code != 0 || errText != "" || out != "Hello" {
		t.Fatalf("%d %q %q", code, out, errText)
	}
	code, out, errText = runOut(t, Options{File: "a.la", Literal: "msg"})
	if code != 0 || errText != "" || !strings.Contains(out, "@<msg@=") {
		t.Fatalf("literal %d %q %q", code, out, errText)
	}
	code, out, errText = runOut(t, Options{File: "a.la", Print: "missing"})
	if code == 0 || out != "" || !strings.Contains(errText, "chunk 'missing' is not defined") {
		t.Fatalf("missing %d %q %q", code, out, errText)
	}
	code, out, errText = runOut(t, Options{File: "a.la", Depend: "nope"})
	if code != 0 || out != "" || errText != "" {
		t.Fatalf("depend %d %q %q", code, out, errText)
	}
	code, out, errText = runOut(t, Options{File: "none.la", Print: "msg"})
	if code == 0 || out != "" || !strings.Contains(errText, "could not find archive 'none.la'") {
		t.Fatalf("missing archive %d %q %q", code, out, errText)
	}
}

func TestFindArchive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	writeFile(t, filepath.Join(dir, "base.la"), "@<msg@=base@\n")
	if err := os.MkdirAll(filepath.Join(sub, "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "deep", "child.la"), "@<msg@+child@\n")
	writeFile(t, filepath.Join(dir, "composer.json"), composerDoc(`[
    {"name":"base","archive":"./base.la","description":"","mappings":[],"base":[]},
    {"name":"child","archive":"./sub/deep/child.la","description":"","mappings":[],"base":["base"]}
]`))
	writeFile(t, filepath.Join(sub, "other.la"), "@<msg@=other@\n")
	writeFile(t, filepath.Join(sub, "composer.json"), composerDoc(`[
    {"name":"other","archive":"./other.la","description":"","mappings":[],"base":[]}
]`))

	w, node, err := FindArchive(filepath.Join(sub, "deep", "child.la"))
	if err != nil || node != "base/child" || w.Composer != filepath.Join(dir, "composer.json") {
		t.Fatalf("child %v %q", err, node)
	}
	_, node, err = FindArchive(filepath.Join(sub, "other.la"))
	if err != nil || node != "other" {
		t.Fatalf("nearest %v %q", err, node)
	}
	stray := filepath.Join(sub, "stray.la")
	writeFile(t, stray, "@<msg@=stray@\n")
	if _, _, err := FindArchive(stray); err == nil || !strings.Contains(err.Error(), "no composition declares archive") {
		t.Fatal(err)
	}
	if _, _, err := FindArchive(filepath.Join(dir, "none.la")); err == nil || !strings.Contains(err.Error(), "could not find archive") {
		t.Fatal(err)
	}

	code, out, errText := runOut(t, Options{File: filepath.Join(sub, "deep", "child.la"), Print: "msg"})
	if code != 0 || errText != "" || out != "basechild" {
		t.Fatalf("%d %q %q", code, out, errText)
	}
}
