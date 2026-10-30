package litar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilterIO(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip(err)
	}
	src := "" +
		"@<upper@=\n" +
		"#!/bin/sh\n" +
		"tr '[:lower:]' '[:upper:]'\n" +
		"@\n" +
		"@<msg@=\n" +
		"hello\n" +
		"@|upper@\n" +
		"@<bad@=\n" +
		"echo hi\n" +
		"@\n" +
		"@<die@=\n" +
		"#!/bin/sh\n" +
		"exit 3\n" +
		"@\n" +
		"@<copy@=\n" +
		"#!/bin/sh\n" +
		"cat\n" +
		"@\n"
	blocks := mustArch(t, src)
	ev := NewEvaluator(blocks, nil, nil)
	got, err := ev.Eval(mustRef(t, "msg"), nil)
	if err != nil || got != "HELLO\n" {
		t.Fatalf("%v %q", err, got)
	}
	// A chunk body is not a filter until a pipeline names it.
	src2 := src + "@<use@=\n" +
		"hi\n" +
		"@|bad@\n" +
		"@<use2@=\n" +
		"hi\n" +
		"@|die@\n" +
		"@<use3@=\n" +
		"hi\n" +
		"@|copy@\n"
	blocks = mustArch(t, src2)
	ev = NewEvaluator(blocks, nil, nil)
	_, err = ev.Eval(mustRef(t, "use"), nil)
	if err == nil || !strings.Contains(err.Error(), "filter 'bad' has no shebang") {
		t.Fatal(err)
	}
	_, err = ev.Eval(mustRef(t, "use2"), nil)
	if err == nil || !strings.Contains(err.Error(), "filter 'die' exited with status 3") {
		t.Fatal(err)
	}
	got, err = ev.Eval(mustRef(t, "use3"), nil)
	if err != nil || got != "hi\n" {
		t.Fatalf("copy %v %q", err, got)
	}
	_, err = ev.Eval(mustRef(t, "missing"), nil)
	if err == nil || !strings.Contains(err.Error(), "not defined") {
		t.Fatal(err)
	}
}

func TestFUSEView(t *testing.T) {
	if _, err := os.Stat("/dev/fuse"); err != nil {
		t.Skip("no /dev/fuse")
	}
	src := "" +
		"@[hello.txt@`docs@=\n" +
		"hello world\n" +
		"@\n" +
		"@(run.sh@`docs@=\n" +
		"echo run\n" +
		"@\n" +
		"@<show@=\n" +
		"#!/bin/sh\n" +
		"cat hello.txt\n" +
		"@\n" +
		"@<msg@=\n" +
		"@|show@+docs@\n" +
		"@<exe@=\n" +
		"#!/bin/sh\n" +
		"if test -x run.sh; then echo yes; else echo no; fi\n" +
		"@\n" +
		"@<ask@=\n" +
		"@|exe@+docs@\n" +
		"@<make@=\n" +
		"#!/bin/sh\n" +
		"printf overlay > created.txt\n" +
		"cat created.txt\n" +
		"@\n" +
		"@<made@=\n" +
		"@|make@\n" +
		"@<ro@=\n" +
		"#!/bin/sh\n" +
		"echo x > hello.txt\n" +
		"@\n" +
		"@<row@=\n" +
		"@|ro@,hello.txt@\n"
	blocks := mustArch(t, src)
	ev := NewEvaluator(blocks, nil, nil)
	got, err := ev.Eval(mustRef(t, "msg"), nil)
	if err != nil || got != "hello world\n" {
		t.Fatalf("view %v %q", err, got)
	}
	got, err = ev.Eval(mustRef(t, "ask"), nil)
	if err != nil || got != "yes\n" {
		t.Fatalf("exec %v %q", err, got)
	}
	got, err = ev.Eval(mustRef(t, "made"), nil)
	if err != nil || got != "overlay" {
		t.Fatalf("overlay %v %q", err, got)
	}
	_, err = ev.Eval(mustRef(t, "row"), nil)
	if err == nil || !strings.Contains(err.Error(), "exited with status") {
		t.Fatal(err)
	}
	ref, filters := mustPrint(t, "@|show@,missing")
	_, err = ev.Eval(ref, filters)
	if err == nil || !strings.Contains(err.Error(), "file 'missing' is not defined") {
		t.Fatal(err)
	}
	ref, filters = mustPrint(t, "@|show@,../x")
	_, err = ev.Eval(ref, filters)
	if err == nil || !strings.Contains(err.Error(), "invalid file path '../x'") {
		t.Fatal(err)
	}
}

func mustPrint(t *testing.T, s string) (ChunkRef, []Filter) {
	t.Helper()
	ref, filters, err := ParsePrint(s)
	if err != nil {
		t.Fatal(err)
	}
	return ref, filters
}

func TestMappedFileAndCycle(t *testing.T) {
	if _, err := os.Stat("/dev/fuse"); err != nil {
		t.Skip("no /dev/fuse")
	}
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.txt")
	writeFile(t, origin, "mapped\n")
	src := "" +
		"@<show@=\n" +
		"#!/bin/sh\n" +
		"cat data.txt\n" +
		"@\n" +
		"@<top@=\n" +
		"@|show@,data.txt@\n" +
		"@<a@=\n" +
		"@<b@>\n" +
		"@\n" +
		"@[b@=\n" +
		"@<a@>\n" +
		"@\n" +
		"@<run@=\n" +
		"#!/bin/sh\n" +
		"cat b\n" +
		"@\n" +
		"@<loop@=\n" +
		"@|run@,b@\n"
	blocks := mustArch(t, src)
	ev := NewEvaluator(blocks, []Mapping{{
		Path:    "data.txt",
		FileSet: []string{"data"},
		Origin:  origin,
	}}, nil)
	got, err := ev.Eval(mustRef(t, "top"), nil)
	if err != nil || got != "mapped\n" {
		t.Fatalf("map %v %q", err, got)
	}
	_, err = ev.Eval(mustRef(t, "loop"), nil)
	if err == nil || !strings.Contains(err.Error(), "circular reference b -> a -> b") {
		t.Fatal(err)
	}
	both := mustArch(t, "@[data.txt@=no@\n")
	ev = NewEvaluator(append(both, blocks...), []Mapping{{
		Path:   "data.txt",
		Origin: origin,
	}}, nil)
	_, err = ev.Eval(mustRef(t, "top"), nil)
	if err == nil || !strings.Contains(err.Error(), "both an archive file and a mapping") {
		t.Fatal(err)
	}
}
