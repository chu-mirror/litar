package litar

import (
	"bytes"
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
		"@<size@=\n" +
		"#!/bin/sh\n" +
		"stat -c %s hello.txt\n" +
		"@\n" +
		"@<sized@=\n" +
		"@|size@,hello.txt@\n" +
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
	got, err = ev.Eval(mustRef(t, "sized"), nil)
	if err != nil || strings.TrimSpace(got) != "12" {
		t.Fatalf("size %v %q", err, got)
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

func TestArchiveFileStatStable(t *testing.T) {
	if _, err := os.Stat("/dev/fuse"); err != nil {
		t.Skip("no /dev/fuse")
	}
	src := "" +
		"@[hello.txt@=\n" +
		"hello world\n" +
		"@\n" +
		"@[doc/note.txt@=\n" +
		"note\n" +
		"@\n" +
		"@<probe@=\n" +
		"#!/bin/sh\n" +
		"fields='%i %s %f %h %X %Y %Z'\n" +
		"s1=$(stat -c \"$fields\" hello.txt)\n" +
		"cat hello.txt >/dev/null\n" +
		"out=$(mktemp)\n" +
		"cp hello.txt \"$out\" || exit 1\n" +
		"cmp hello.txt \"$out\" || exit 1\n" +
		"rm -f \"$out\"\n" +
		"s2=$(stat -c \"$fields\" hello.txt)\n" +
		"d1=$(stat -c %i doc)\n" +
		"cat doc/note.txt >/dev/null\n" +
		"d2=$(stat -c %i doc)\n" +
		"n1=$(stat -c \"$fields\" doc/note.txt)\n" +
		"if [ \"$s1\" != \"$s2\" ] || [ \"$d1\" != \"$d2\" ]; then\n" +
		"  echo \"changed $s1 $s2 $d1 $d2\"\n" +
		"  exit 1\n" +
		"fi\n" +
		"if touch hello.txt || chmod 644 hello.txt; then\n" +
		"  echo mutable\n" +
		"  exit 1\n" +
		"fi\n" +
		"s3=$(stat -c \"$fields\" hello.txt)\n" +
		"if [ \"$s1\" != \"$s3\" ]; then\n" +
		"  echo \"meta $s1 $s3\"\n" +
		"  exit 1\n" +
		"fi\n" +
		"echo \"$s1\"\n" +
		"echo \"$n1\"\n" +
		"@\n" +
		"@<run@=\n" +
		"@|probe@,hello.txt@,doc/note.txt@\n"
	var errb bytes.Buffer
	ev := NewEvaluator(mustArch(t, src), nil, &errb)
	got, err := ev.Eval(mustRef(t, "run"), nil)
	if err != nil {
		t.Fatalf("%v\n%s\n%s", err, got, errb.String())
	}
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 2 {
		t.Fatalf("%q", got)
	}
	hello := strings.Fields(lines[0])
	note := strings.Fields(lines[1])
	// ino, size, mode, nlink, atime, mtime, ctime
	if len(hello) != 7 || len(note) != 7 {
		t.Fatalf("%q", got)
	}
	if hello[0] == note[0] || hello[0] == "0" {
		t.Fatalf("inode %q", got)
	}
	if hello[1] != "12" || note[1] != "5" {
		t.Fatalf("size %q", got)
	}
	if hello[3] != "1" || note[3] != "1" {
		t.Fatalf("nlink %q", got)
	}
	if hello[4] != "0" || hello[5] != "0" || hello[6] != "0" {
		t.Fatalf("time %q", got)
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
