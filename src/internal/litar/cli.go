package litar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const missingAction = "missing -p, --print, -l, --literal, -e, --effective, -d, --depend, -r, --reference, or -a, --agent"

var modeOrder = []string{
	"print", "literal", "effective", "depend", "reference", "agent",
}

// Options is the command line of litar after parsing.
// An action is given by a non-empty operand; Agent takes none.
type Options struct {
	Agent     bool
	File      string
	Literal   string
	Effective string
	Depend    string
	Reference string
	Print     string
}

// Main runs litar with a parsed command line and returns the exit status.
func Main(opts Options) int {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-sigs
		DetachMounts()
		os.Exit(128 + int(sig.(syscall.Signal)))
	}()
	filterCacheDir = cacheDirFromEnv()
	return run(opts, os.Stdout, os.Stderr)
}

func run(opts Options, stdout, stderr io.Writer) int {
	fail := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "Error: "+format+"\n", a...)
		return 1
	}
	var mode, operand string
	operands := map[string]string{
		"print":     opts.Print,
		"literal":   opts.Literal,
		"effective": opts.Effective,
		"depend":    opts.Depend,
		"reference": opts.Reference,
	}
	for _, name := range modeOrder {
		given := operands[name] != ""
		if name == "agent" {
			given = opts.Agent
		}
		if !given {
			continue
		}
		if mode != "" {
			return fail("cannot combine %s and %s", mode, name)
		}
		mode, operand = name, operands[name]
	}
	if mode == "" {
		return fail("%s", missingAction)
	}

	archive := opts.File
	sawFile := archive != ""
	if mode != "agent" && !sawFile {
		return fail("missing -f, --file")
	}

	var (
		world *World
		node  string
	)
	if sawFile {
		var err error
		world, node, err = FindArchive(archive)
		if err != nil {
			return fail("%s", err.Error())
		}
	}

	if mode == "agent" {
		ag := newAgent()
		if world != nil {
			ag.addWorld(world)
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
