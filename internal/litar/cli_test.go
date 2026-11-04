package litar

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runOut(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, err bytes.Buffer
	code := run(args, &out, &err)
	return code, out.String(), err.String()
}

func TestHelp(t *testing.T) {
	code, out, err := runOut(t, "litar", "--help", "-p")
	if code != 0 || err != "" || out != usage("litar") {
		t.Fatalf("code %d err %q out %q", code, err, out)
	}
	if !strings.HasSuffix(out, "-p, --print EXPRESSION: evaluate EXPRESSION, and print it\n") {
		t.Fatalf("help ending %q", out)
	}
	code, out, err = runOut(t, "litar", "-h")
	if code == 0 || out != "" || !strings.Contains(err, "unknown option '-h'") {
		t.Fatalf("%d %q %q", code, out, err)
	}
}

func TestCLIErrors(t *testing.T) {
	cases := []struct {
		args []string
		msg  string
	}{
		{[]string{"litar"}, missingAction},
		{[]string{"litar", "-p", "a", "-l", "a"}, "cannot combine print and literal"},
		{[]string{"litar", "-l", "a", "-p", "b"}, "cannot combine print and literal"},
		{[]string{"litar", "-p", "a", "-p", "b"}, "duplicate print option"},
		{[]string{"litar", "-f", "a", "-f", "b", "-p", "c"}, "duplicate file option"},
		{[]string{"litar", "-p", "msg"}, "missing -f, --file"},
		{[]string{"litar", "-t", ".", "-p", "msg"}, "unknown option '-t'"},
		{[]string{"litar", "-al"}, "unknown option '-al'"},
		{[]string{"litar", "archive.la"}, "unexpected argument 'archive.la'"},
	}
	for _, tc := range cases {
		code, out, err := runOut(t, tc.args...)
		if code == 0 || out != "" || !strings.Contains(err, tc.msg) || !strings.HasPrefix(err, "Error: ") {
			t.Fatalf("%v code %d out %q err %q", tc.args, code, out, err)
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
	code, out, errText := runOut(t, "litar", "-f", "a.la", "-p", "msg")
	if code != 0 || errText != "" || out != "Hello" {
		t.Fatalf("%d %q %q", code, out, errText)
	}
	code, out, errText = runOut(t, "litar", "-fa.la", "-l", "msg")
	if code != 0 || errText != "" || !strings.Contains(out, "@<msg@=") {
		t.Fatalf("literal %d %q %q", code, out, errText)
	}
	code, out, errText = runOut(t, "litar", "--file=a.la", "-p", "missing")
	if code == 0 || out != "" || !strings.Contains(errText, "chunk 'missing' is not defined") {
		t.Fatalf("missing %d %q %q", code, out, errText)
	}
	code, out, errText = runOut(t, "litar", "-f", "a.la", "-d", "nope")
	if code != 0 || out != "" || errText != "" {
		t.Fatalf("depend %d %q %q", code, out, errText)
	}
	code, out, errText = runOut(t, "litar", "-f", "a.la", "-m", "/")
	if code == 0 || out != "" || !strings.Contains(errText, "unknown option '-m'") {
		t.Fatalf("modules %d %q %q", code, out, errText)
	}
	code, out, errText = runOut(t, "litar", "-f", "none.la", "-p", "msg")
	if code == 0 || out != "" || !strings.Contains(errText, "could not find archive 'none.la'") {
		t.Fatalf("missing archive %d %q %q", code, out, errText)
	}
	code, out, errText = runOut(t, "litar", "-pfoo", "-f", "a.la")
	if code != 0 || errText != "" || out != "" {
		// foo is not defined. Attached value is the expression.
		if code == 0 || !strings.Contains(errText, "chunk 'foo' is not defined") {
			t.Fatalf("attached %d %q %q", code, out, errText)
		}
	}
}

func TestFindArchive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "base.la"), "@<msg@=base@\n")
	sub := filepath.Join(dir, "sub")
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

	code, out, errText := runOut(t, "litar", "-f", filepath.Join(sub, "deep", "child.la"), "-p", "msg")
	if code != 0 || errText != "" || out != "basechild" {
		t.Fatalf("%d %q %q", code, out, errText)
	}
}
