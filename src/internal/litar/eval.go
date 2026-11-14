package litar

import (
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"sync"
)

// fileCache holds the files evaluated in this process, by path.
var fileCache = struct {
	sync.Mutex
	files map[string]cachedFile
}{files: map[string]cachedFile{}}

type cycleError struct {
	chain []ChunkRef
}

type frame struct {
	key string
	ref ChunkRef
}

// Evaluator expands chunk references on one node chain.
// The mutex is held across in-memory expansion and released around
// filter execution, which may mount a filesystem and block.
// deps holds the dependencies of each file being expanded, innermost last.
type Evaluator struct {
	mu      sync.Mutex
	chain   []Block
	cat     *catalog
	stack   []frame
	deps    []map[string]dependency
	fuseErr error
	stderr  io.Writer
}

type cachedFile struct {
	text string
	deps map[string]dependency
}

type dependency struct {
	ref    ChunkRef
	view   []ViewOp
	isView bool
	sum    [sha256.Size]byte
}

func (e *cycleError) Error() string {
	parts := make([]string, len(e.chain))
	for i, ref := range e.chain {
		parts[i] = ref.String()
	}
	return "circular reference " + strings.Join(parts, " -> ")
}

func (e *Evaluator) noteErr(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fuseErr == nil && err != nil {
		e.fuseErr = err
	}
}

func (e *Evaluator) takeFuseErr() error {
	err := e.fuseErr
	e.fuseErr = nil
	return err
}

// Eval evaluates a print expression. A missing chunk is an error.
// An empty chunk with a pipeline is the empty input of that pipeline.
func (e *Evaluator) Eval(ref ChunkRef, filters []Filter) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.evalLocked(ref, filters)
}

func (e *Evaluator) evalLocked(ref ChunkRef, filters []Filter) (string, error) {
	var (
		text string
		err  error
	)
	if ref.Chunk == "" {
		text = ""
	} else {
		text, err = e.expandLocked(ref, true)
		if err != nil {
			return "", err
		}
	}
	if len(filters) == 0 {
		return text, nil
	}
	return e.runFiltersLocked(text, filters, ref)
}

func (e *Evaluator) expandLocked(ref ChunkRef, missingErr bool) (string, error) {
	if !e.isArchiveFile(ref) {
		return e.expandChunkLocked(ref, missingErr)
	}
	fileCache.Lock()
	cached, ok := fileCache.files[string(ref.Chunk)]
	fileCache.Unlock()
	if ok && e.unchanged(cached.deps) {
		e.addDeps(cached.deps)
		return cached.text, nil
	}
	return e.expandFileLocked(ref, missingErr)
}

func (e *Evaluator) expandFileLocked(ref ChunkRef, missingErr bool) (string, error) {
	e.deps = append(e.deps, map[string]dependency{})
	text, err := e.expandChunkLocked(ref, missingErr)
	deps := e.deps[len(e.deps)-1]
	e.deps = e.deps[:len(e.deps)-1]
	if err != nil {
		return "", err
	}
	e.addDeps(deps)
	fileCache.Lock()
	fileCache.files[string(ref.Chunk)] = cachedFile{text: text, deps: deps}
	fileCache.Unlock()
	return text, nil
}

func (e *Evaluator) expandChunkLocked(ref ChunkRef, missingErr bool) (string, error) {
	key := ref.key()
	for i, fr := range e.stack {
		if fr.key == key {
			chain := make([]ChunkRef, 0, len(e.stack)-i+1)
			for _, older := range e.stack[i:] {
				chain = append(chain, older.ref)
			}
			chain = append(chain, ref)
			return "", &cycleError{chain: chain}
		}
	}
	e.recordChunk(key, ref)
	e.stack = append(e.stack, frame{key: key, ref: ref})
	defer func() { e.stack = e.stack[:len(e.stack)-1] }()

	var acc string
	matched := false
	for _, b := range e.chain {
		if !matchRef(b.Ref, ref) {
			continue
		}
		matched = true
		text, err := e.expandBlock(b, ref)
		if err != nil {
			return "", err
		}
		switch {
		case b.Op == OpSet:
			acc = text
		case b.EndsLine && acc != "":
			acc += "\n" + text
		default:
			acc += text
		}
	}
	if !matched {
		if missingErr {
			return "", fmt.Errorf("chunk '%s' is not defined", ref.String())
		}
		return "", nil
	}
	return acc, nil
}

// isArchiveFile reports whether ref names a file of the archive:
// a chunk of the root module, without labels, that a file block defines.
func (e *Evaluator) isArchiveFile(ref ChunkRef) bool {
	if len(ref.Labels) > 0 || len(ref.Module) > 0 {
		return false
	}
	f := e.cat.files[string(ref.Chunk)]
	return f != nil && f.archive
}

// recording is the dependencies of the innermost file being expanded,
// or nil when no file is.
func (e *Evaluator) recording() map[string]dependency {
	if len(e.deps) == 0 {
		return nil
	}
	return e.deps[len(e.deps)-1]
}

func (e *Evaluator) addDeps(deps map[string]dependency) {
	into := e.recording()
	if into == nil {
		return
	}
	for key, dep := range deps {
		into[key] = dep
	}
}

func (e *Evaluator) recordChunk(key string, ref ChunkRef) {
	if deps := e.recording(); deps != nil {
		deps["chunk "+key] = dependency{ref: ref, sum: e.chunkSum(ref)}
	}
}

func (e *Evaluator) recordView(view []ViewOp) {
	deps := e.recording()
	if deps == nil {
		return
	}
	var key strings.Builder
	key.WriteString("view")
	for _, op := range view {
		fmt.Fprintf(&key, " %d:%q", op.Kind, op.Name)
	}
	deps[key.String()] = dependency{view: view, isView: true, sum: e.viewSum(view)}
}

func (e *Evaluator) unchanged(deps map[string]dependency) bool {
	for _, dep := range deps {
		var sum [sha256.Size]byte
		if dep.isView {
			sum = e.viewSum(dep.view)
		} else {
			sum = e.chunkSum(dep.ref)
		}
		if sum != dep.sum {
			return false
		}
	}
	return true
}

func (e *Evaluator) chunkSum(ref ChunkRef) [sha256.Size]byte {
	h := sha256.New()
	for _, b := range e.chain {
		if matchRef(b.Ref, ref) {
			fmt.Fprintf(h, "%d:%s", len(b.Raw), b.Raw)
		}
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

func (e *Evaluator) viewSum(view []ViewOp) [sha256.Size]byte {
	h := sha256.New()
	files, err := e.cat.apply(view)
	if err != nil {
		fmt.Fprintf(h, "error %s\n", err)
	}
	for _, f := range files {
		fmt.Fprintf(h, "%q %t %t %t %q\n", f.path, f.archive, f.executable, f.dir, f.origin)
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

func (e *Evaluator) expandBlock(b Block, applied ChunkRef) (string, error) {
	var sb strings.Builder
	indent, blank := "", true
	for _, part := range b.Parts {
		if part.Ref == nil {
			sb.WriteString(part.Text)
			indent, blank = continueLine(indent, blank, part.Text)
			continue
		}
		inner := resolveBody(part.Ref.ChunkRef, applied.Labels, b.Ref.Module)
		text, err := e.expandLocked(inner, false)
		if err != nil {
			return "", err
		}
		if len(part.Ref.Filters) > 0 {
			text, err = e.runFiltersLocked(text, part.Ref.Filters, inner)
			if err != nil {
				return "", err
			}
		}
		if blank && indent != "" {
			text = indentLines(text, indent)
		}
		blank = false
		sb.WriteString(text)
	}
	text := sb.String()
	if len(b.Filters) > 0 {
		var err error
		text, err = e.runFiltersLocked(text, b.Filters, applied)
		if err != nil {
			return "", err
		}
	}
	return text, nil
}

func (e *Evaluator) runFiltersLocked(text string, filters []Filter, attached ChunkRef) (string, error) {
	for _, f := range filters {
		resolved := resolveFilter(f.Ref, attached.Labels)
		script, err := e.expandLocked(resolved, true)
		if err != nil {
			return "", err
		}
		e.recordView(f.View)
		e.mu.Unlock()
		out, runErr := executeFilter(e, script, text, resolved, f.View)
		e.mu.Lock()
		if fuse := e.takeFuseErr(); fuse != nil {
			return "", fuse
		}
		if runErr != nil {
			return "", runErr
		}
		text = out
	}
	return text, nil
}

// readChunk evaluates a root-module file chunk on the current visit stack.
// Tangling calls it before the filter process starts, so the lock
// is not already held.
func (e *Evaluator) readChunk(chunk Name) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	text, err := e.expandLocked(ChunkRef{Chunk: chunk}, false)
	if err != nil {
		if e.fuseErr == nil {
			e.fuseErr = err
		}
		return nil, err
	}
	return []byte(text), nil
}

func NewEvaluator(chain []Block, mappings []Mapping, stderr io.Writer) *Evaluator {
	if stderr == nil {
		stderr = io.Discard
	}
	return &Evaluator{
		chain:  chain,
		cat:    buildCatalog(chain, mappings),
		stderr: stderr,
	}
}

func resolveBody(ref ChunkRef, outerLabels []Name, header ModulePath) ChunkRef {
	labels := ref.Labels
	if len(labels) == 0 {
		labels = copyLabels(outerLabels)
	} else {
		labels = copyLabels(labels)
	}
	var mod ModulePath
	if hasRel(ref.Module) {
		mod = resolveModule(header, ref.Module)
	} else {
		mod = copyModule(ref.Module)
	}
	return ChunkRef{Labels: labels, Module: mod, Chunk: ref.Chunk}
}

func resolveFilter(ref ChunkRef, outerLabels []Name) ChunkRef {
	labels := ref.Labels
	if len(labels) == 0 {
		labels = copyLabels(outerLabels)
	} else {
		labels = copyLabels(labels)
	}
	return ChunkRef{Labels: labels, Module: copyModule(ref.Module), Chunk: ref.Chunk}
}

// continueLine adds text to a line of a block body: indent is the line so far
// and blank tells that it holds only spaces and tabs. It returns the same
// for the line that text ends in.
func continueLine(indent string, blank bool, text string) (string, bool) {
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		indent, blank, text = "", true, text[i+1:]
	}
	if !blank || strings.Trim(text, " \t") != "" {
		return "", false
	}
	return indent + text, true
}

// indentLines writes indent before every line of text after the first.
// An empty line stays empty, and nothing follows a final line break.
func indentLines(text, indent string) string {
	var sb strings.Builder
	for {
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			break
		}
		sb.WriteString(text[:i+1])
		text = text[i+1:]
		if text != "" && text[0] != '\n' && text[0] != '\r' {
			sb.WriteString(indent)
		}
	}
	sb.WriteString(text)
	return sb.String()
}
