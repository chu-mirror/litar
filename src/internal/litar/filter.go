package litar

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type catFile struct {
	sets       map[string]struct{}
	executable bool
	archive    bool
	mapping    bool
	// dir is set for a mapping of a directory.
	dir    bool
	origin string
}

type catalog struct {
	order     []string
	files     map[string]*catFile
	conflicts map[string]bool
}

type viewFile struct {
	path       string
	executable bool
	origin     string
	archive    bool
	dir        bool
	body       []byte
	// ino is assigned once for the mount. A fresh number on each
	// lookup makes tools treat the file as replaced mid-invocation.
	ino uint64
}

func (c *catalog) apply(ops []ViewOp) ([]viewFile, error) {
	in := map[string]bool{}
	var order []string
	add := func(path string) error {
		if err := validFilePath(path); err != nil {
			return err
		}
		f := c.files[path]
		if f == nil || (!f.archive && !f.mapping) {
			return fmt.Errorf("file '%s' is not defined", path)
		}
		if f.archive && f.mapping || c.conflicts[path] {
			return fmt.Errorf("file '%s' is defined as both an archive file and a mapping", path)
		}
		if !in[path] {
			in[path] = true
			order = append(order, path)
		}
		return nil
	}
	for _, op := range ops {
		switch op.Kind {
		case AddSet:
			name := string(op.Name)
			for _, path := range c.order {
				f := c.files[path]
				if _, ok := f.sets[name]; !ok {
					continue
				}
				if err := add(path); err != nil {
					return nil, err
				}
			}
		case SubSet:
			name := string(op.Name)
			for _, path := range c.order {
				f := c.files[path]
				if _, ok := f.sets[name]; ok {
					delete(in, path)
				}
			}
		case AddFile:
			if err := add(string(op.Name)); err != nil {
				return nil, err
			}
		case DelFile:
			path := string(op.Name)
			if err := validFilePath(path); err != nil {
				return nil, err
			}
			delete(in, path)
		}
	}
	var out []viewFile
	seen := map[string]bool{}
	for _, path := range order {
		if !in[path] || seen[path] {
			continue
		}
		seen[path] = true
		f := c.files[path]
		out = append(out, viewFile{
			path:       path,
			executable: f.executable && f.archive,
			origin:     f.origin,
			archive:    f.archive,
			dir:        f.dir && !f.archive,
		})
	}
	return out, nil
}

// tangle writes each view file's bytes before the filter process starts.
// An archive file is the root-module chunk named by its path.
// A mapped file is the contents of origin. A mapped directory is created
// when it is missing, so a build can write into a fresh tree.
func (e *Evaluator) tangle(files []viewFile) error {
	for i := range files {
		var (
			data []byte
			err  error
		)
		switch {
		case files[i].archive:
			data, err = e.readChunk(Name(files[i].path))
		case files[i].dir:
			if err = os.MkdirAll(files[i].origin, 0o755); err != nil {
				err = fmt.Errorf("could not create '%s'", files[i].origin)
				e.noteErr(err)
			}
		default:
			data, err = os.ReadFile(files[i].origin)
			if err != nil {
				err = fmt.Errorf("could not read '%s'", files[i].origin)
				e.noteErr(err)
			}
		}
		if err != nil {
			return err
		}
		files[i].body = data
	}
	return nil
}

func buildCatalog(chain []Block, mappings []Mapping) *catalog {
	c := &catalog{
		files:     map[string]*catFile{},
		conflicts: map[string]bool{},
	}
	touch := func(path string) *catFile {
		f := c.files[path]
		if f == nil {
			f = &catFile{sets: map[string]struct{}{}}
			c.files[path] = f
			c.order = append(c.order, path)
		}
		return f
	}
	for _, b := range chain {
		if !b.IsFile {
			continue
		}
		f := touch(string(b.Path))
		f.archive = true
		f.executable = b.Executable
		for _, set := range b.Sets {
			f.sets[string(set)] = struct{}{}
		}
	}
	for _, m := range mappings {
		path, dir := strings.CutSuffix(m.Path, "/")
		f := touch(path)
		if f.archive {
			c.conflicts[path] = true
		}
		f.mapping = true
		f.dir = dir
		f.origin = m.Origin
		for _, set := range m.FileSet {
			f.sets[set] = struct{}{}
		}
	}
	return c
}

func validFilePath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\x00") {
		return fmt.Errorf("invalid file path '%s'", p)
	}
	for _, c := range strings.Split(p, "/") {
		if c == "" || c == "." || c == ".." {
			return fmt.Errorf("invalid file path '%s'", p)
		}
	}
	return nil
}

// parseShebang reads "#! interpreter" or "#! interpreter arg".
// As the kernel's script loader does, arg is the rest of the line,
// one argument without its surrounding blanks.
func parseShebang(script string) (interp, arg string, hasArg, ok bool) {
	if len(script) < 2 || script[0] != '#' || script[1] != '!' {
		return "", "", false, false
	}
	i := 2
	for i < len(script) && (script[i] == ' ' || script[i] == '\t') {
		i++
	}
	interp, i = shebangWord(script, i)
	if interp == "" {
		return "", "", false, false
	}
	if i < len(script) && script[i] == '\r' {
		i++
	}
	if i >= len(script) || script[i] == '\n' {
		return interp, "", false, true
	}
	for i < len(script) && (script[i] == ' ' || script[i] == '\t') {
		i++
	}
	if i >= len(script) || script[i] == '\n' || script[i] == '\r' {
		return interp, "", false, true
	}
	end := strings.IndexByte(script[i:], '\n')
	if end < 0 {
		end = len(script) - i
	}
	arg = strings.TrimRight(script[i:i+end], " \t\r")
	return interp, arg, true, true
}

// shebangWord returns the word of script at i, up to a blank or a line end,
// and the index after it.
func shebangWord(script string, i int) (string, int) {
	start := i
	for i < len(script) && script[i] != ' ' && script[i] != '\t' && script[i] != '\n' && script[i] != '\r' {
		i++
	}
	return stripCR(script[start:i]), i
}

func stripCR(s string) string {
	if strings.HasSuffix(s, "\r") {
		return strings.TrimSuffix(s, "\r")
	}
	return s
}

func executeFilter(ev *Evaluator, script, input string, ref ChunkRef, view []ViewOp) (string, error) {
	interp, arg, hasArg, ok := parseShebang(script)
	if !ok {
		return "", fmt.Errorf("filter '%s' has no shebang", ref.String())
	}
	files, err := ev.cat.apply(view)
	if err != nil {
		return "", err
	}
	if err := ev.tangle(files); err != nil {
		return "", err
	}
	var scriptPath string
	scriptFile, err := os.CreateTemp("", "litar-script-*")
	if err != nil {
		return "", fmt.Errorf("filter '%s' exited with status 1", ref.String())
	}
	scriptPath = scriptFile.Name()
	if _, err := scriptFile.WriteString(script); err != nil {
		scriptFile.Close()
		os.Remove(scriptPath)
		return "", fmt.Errorf("filter '%s' exited with status 1", ref.String())
	}
	scriptFile.Close()
	defer os.Remove(scriptPath)

	mnt, err := mountView(files)
	if err != nil {
		return "", err
	}
	defer mnt.close()

	args := []string{interp, scriptPath}
	if hasArg {
		args = []string{interp, arg, scriptPath}
	}
	// The child must not enter the mount before exec. Until exec it shares
	// memory with this process, whose FUSE server answers the chdir;
	// a stop-the-world in between waits for the forking thread forever.
	cmd := exec.Command("/bin/sh", append([]string{"-c", `cd "$0" && exec "$@"`, mnt.dir}, args...)...)
	cmd.Stderr = ev.stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("filter '%s' exited with status 1", ref.String())
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("filter '%s' exited with status 1", ref.String())
	}
	if err := cmd.Start(); err != nil {
		code := 1
		if os.IsNotExist(err) {
			code = 127
		}
		return "", fmt.Errorf("filter '%s' exited with status %d", ref.String(), code)
	}
	var out bytes.Buffer
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&out, stdout)
		close(copyDone)
	}()
	_, _ = io.Copy(stdin, strings.NewReader(input))
	_ = stdin.Close()
	waitErr := cmd.Wait()
	<-copyDone
	if waitErr != nil {
		return "", fmt.Errorf("filter '%s' exited with status %d", ref.String(), exitStatus(waitErr))
	}
	return out.String(), nil
}

// exitStatus is the status of a failed wait: the exit code,
// or 128 plus the signal for a process killed by a signal.
func exitStatus(err error) int {
	code := 1
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
		if code < 0 {
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				code = 128 + int(ws.Signal())
			} else {
				code = 1
			}
		}
	}
	return code
}
