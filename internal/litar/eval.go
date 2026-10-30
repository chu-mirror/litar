package litar

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

type cycleError struct {
	chain []ChunkRef
}

func (e *cycleError) Error() string {
	parts := make([]string, len(e.chain))
	for i, ref := range e.chain {
		parts[i] = ref.String()
	}
	return "circular reference " + strings.Join(parts, " -> ")
}

type frame struct {
	key string
	ref ChunkRef
}

// Evaluator expands chunk references on one node chain.
// The mutex is held across in-memory expansion and released around
// filter execution, which may mount a filesystem and block.
type Evaluator struct {
	mu      sync.Mutex
	chain   []Block
	cat     *catalog
	stack   []frame
	fuseErr error
	stderr  io.Writer
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
		if b.Op == OpSet {
			acc = text
		} else {
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

func (e *Evaluator) expandBlock(b Block, applied ChunkRef) (string, error) {
	var sb strings.Builder
	for _, part := range b.Parts {
		if part.Ref == nil {
			sb.WriteString(part.Text)
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

func (e *Evaluator) runFiltersLocked(text string, filters []Filter, attached ChunkRef) (string, error) {
	for _, f := range filters {
		resolved := resolveFilter(f.Ref, attached.Labels)
		script, err := e.expandLocked(resolved, true)
		if err != nil {
			return "", err
		}
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
// It is called from a FUSE read while a filter is running, so the lock
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
