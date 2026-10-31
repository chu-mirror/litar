package litar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const usageBody = `--help: print the usage
-a, --agent: open an MCP server to handle the requests from agents
-t, --target COMPOSER_DIRECTORY: read in the composer file in COMPOSER_DIRECTORY
-n, --node NODE: specify the NODE
-m, --modules MODULE: list all modules under MODULE
-c, --chunks MODULE: list all chunk names under MODULE
-l, --literal CHUNK_REFERENCE: print the blocks applied to CHUNK_REFERENCE literally
-e, --effective CHUNK_REFERENCE: print the effective blocks of CHUNK_REFERENCE
-d, --depend CHUNK_REFERENCE: list all blocks that depend on CHUNK_REFERENCE
-s, --subdomains DOMAIN: list all subdomains under DOMAIN
-i, --items DOMAIN: list all items under DOMAIN
-r, --reference ITEM: list all blocks that refer to ITEM
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
`

// Main runs litar. args[0] is the program name.
func Main(args []string) int {
	return run(args, os.Stdout, os.Stderr)
}

func usage(prog string) string {
	if prog == "" {
		prog = "litar"
	}
	return "usage: " + prog + " [options]\n\n" + usageBody
}

const missingAction = "missing -p, --print, -l, --literal, -e, --effective, -d, --depend, -m, --modules, -c, --chunks, -s, --subdomains, -i, --items, -r, --reference, or -a, --agent"

var modeOrder = []string{
	"print", "literal", "effective", "depend", "modules", "chunks",
	"subdomains", "items", "reference", "agent",
}

func modeIndex(name string) int {
	for i, m := range modeOrder {
		if m == name {
			return i
		}
	}
	return -1
}

func run(args []string, stdout, stderr io.Writer) int {
	prog := "litar"
	rest := args
	if len(args) > 0 {
		prog = args[0]
		rest = args[1:]
	}
	for _, a := range rest {
		if a == "--help" {
			if _, err := io.WriteString(stdout, usage(prog)); err != nil {
				fmt.Fprintf(stderr, "Error: could not write output\n")
				return 1
			}
			return 0
		}
	}

	var (
		mode      string
		dir       string
		node      string
		operand   string
		sawTarget bool
		sawNode   bool
	)
	fail := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "Error: "+format+"\n", a...)
		return 1
	}
	setMode := func(next string) int {
		if mode == next {
			return fail("duplicate %s option", next)
		}
		if mode != "" {
			first, second := mode, next
			if modeIndex(first) > modeIndex(second) {
				first, second = second, first
			}
			return fail("cannot combine %s and %s", first, second)
		}
		mode = next
		return 0
	}
	need := func(opt, what string, i *int) (string, int) {
		if *i+1 >= len(rest) {
			return "", fail("%s requires %s", opt, what)
		}
		*i++
		return rest[*i], 0
	}

	for i := 0; i < len(rest); i++ {
		a := rest[i]
		opt := a
		val := ""
		hasVal := false
		switch {
		case a == "--":
			return fail("unknown option '%s'", a)
		case strings.HasPrefix(a, "--"):
			name, v, ok := strings.Cut(a, "=")
			opt = name
			if ok {
				val = v
				hasVal = true
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			if len(a) > 2 {
				// Attached value for an option that takes one.
				opt = a[:2]
				val = a[2:]
				hasVal = true
			}
		default:
			return fail("unexpected argument '%s'", a)
		}

		takes := func(what string) (string, int) {
			if hasVal {
				return val, 0
			}
			return need(opt, what, &i)
		}
		// A short cluster that does not take a value is not an option.
		if hasVal && !optionTakesValue(opt) {
			return fail("unknown option '%s'", a)
		}

		var code int
		switch opt {
		case "-t", "--target":
			if sawTarget {
				return fail("duplicate target option")
			}
			dir, code = takes("a composer directory")
			if code != 0 {
				return code
			}
			sawTarget = true
		case "-n", "--node":
			if sawNode {
				return fail("duplicate node option")
			}
			node, code = takes("a node")
			if code != 0 {
				return code
			}
			sawNode = true
		case "-p", "--print":
			if code = setMode("print"); code != 0 {
				return code
			}
			operand, code = takes("an expression")
		case "-l", "--literal":
			if code = setMode("literal"); code != 0 {
				return code
			}
			operand, code = takes("a chunk reference")
		case "-e", "--effective":
			if code = setMode("effective"); code != 0 {
				return code
			}
			operand, code = takes("a chunk reference")
		case "-d", "--depend":
			if code = setMode("depend"); code != 0 {
				return code
			}
			operand, code = takes("a chunk reference")
		case "-m", "--modules":
			if code = setMode("modules"); code != 0 {
				return code
			}
			operand, code = takes("a module")
		case "-c", "--chunks":
			if code = setMode("chunks"); code != 0 {
				return code
			}
			operand, code = takes("a module")
		case "-s", "--subdomains":
			if code = setMode("subdomains"); code != 0 {
				return code
			}
			operand, code = takes("a domain")
		case "-i", "--items":
			if code = setMode("items"); code != 0 {
				return code
			}
			operand, code = takes("a domain")
		case "-r", "--reference":
			if code = setMode("reference"); code != 0 {
				return code
			}
			operand, code = takes("an item")
		case "-a", "--agent":
			if hasVal {
				return fail("unknown option '%s'", a)
			}
			code = setMode("agent")
		default:
			return fail("unknown option '%s'", a)
		}
		if code != 0 {
			return code
		}
	}

	if mode == "" {
		return fail("%s", missingAction)
	}
	if mode != "agent" && !sawTarget {
		return fail("missing -t, --target")
	}

	var world *World
	if sawTarget {
		st, err := os.Stat(dir)
		if err != nil {
			return fail("could not read composer directory '%s': %v", dir, err)
		}
		if !st.IsDir() {
			return fail("target '%s' is not a directory", dir)
		}
		world, err = LoadComposer(filepath.Join(dir, "composer.json"))
		if err != nil {
			return fail("%s", err.Error())
		}
	}
	if node != "" && world != nil {
		if _, err := world.lookup(node); err != nil {
			return fail("%s", err.Error())
		}
	}

	if mode == "agent" {
		ag := newAgent()
		if world != nil {
			ag.world = world
			ag.ensureTools()
			ag.refresh()
		}
		err := ag.serve(context.Background())
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
			return fail("%s", err.Error())
		}
		return 0
	}

	text, err := dispatch(world, node, mode, operand, stderr)
	if err != nil {
		return fail("%s", err.Error())
	}
	if _, err := io.WriteString(stdout, text); err != nil {
		return fail("could not write output")
	}
	return 0
}

func optionTakesValue(opt string) bool {
	switch opt {
	case "-t", "--target", "-n", "--node", "-p", "--print",
		"-l", "--literal", "-e", "--effective", "-d", "--depend",
		"-m", "--modules", "-c", "--chunks", "-s", "--subdomains",
		"-i", "--items", "-r", "--reference":
		return true
	default:
		return false
	}
}

func dispatch(world *World, node, mode, operand string, stderr io.Writer) (string, error) {
	if node == "" {
		var err error
		node, err = world.rootName()
		if err != nil {
			return "", err
		}
	}
	chain, err := world.Chain(node)
	if err != nil {
		return "", err
	}
	switch mode {
	case "print":
		ref, filters, err := ParsePrint(operand)
		if err != nil {
			return "", err
		}
		maps, err := world.Mappings(node)
		if err != nil {
			return "", err
		}
		ev := NewEvaluator(chain, maps, stderr)
		return ev.Eval(ref, filters)
	case "literal":
		ref, err := ParseChunkRef(operand)
		if err != nil {
			return "", err
		}
		return Literal(chain, ref)
	case "effective":
		ref, err := ParseChunkRef(operand)
		if err != nil {
			return "", err
		}
		return Effective(chain, ref)
	case "depend":
		ref, err := ParseChunkRef(operand)
		if err != nil {
			return "", err
		}
		return Depend(chain, ref), nil
	case "modules":
		mod, err := ParseModulePath(operand)
		if err != nil {
			return "", err
		}
		return Modules(chain, mod), nil
	case "chunks":
		mod, err := ParseModulePath(operand)
		if err != nil {
			return "", err
		}
		return Chunks(chain, mod), nil
	case "subdomains":
		dom, err := ParseModulePath(operand)
		if err != nil {
			return "", err
		}
		return Subdomains(chain, dom), nil
	case "items":
		dom, err := ParseModulePath(operand)
		if err != nil {
			return "", err
		}
		return Items(chain, dom), nil
	case "reference":
		item, err := ParseItem(operand)
		if err != nil {
			return "", err
		}
		return Refer(chain, item), nil
	default:
		return "", fmt.Errorf("unknown option '%s'", mode)
	}
}
