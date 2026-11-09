// Command litar prints, lists, and serves the chunks of litar compositions.
package main

import (
	"flag"
	"fmt"
	"litar/internal/litar"
	"os"
)

var command struct {
	flags struct {
		agent     bool
		file      string
		literal   string
		effective string
		depend    string
		reference string
		print     string
	}
}

func main() {
	flag.BoolVar(&command.flags.agent, "agent", false, "open an MCP server to handle the requests from agents")
	flag.BoolVar(&command.flags.agent, "a", false, "open an MCP server to handle the requests from agents")
	flag.StringVar(&command.flags.file, "file", "", "specify the archive file, find the composition and the node corresponding")
	flag.StringVar(&command.flags.file, "f", "", "specify the archive file, find the composition and the node corresponding")
	flag.StringVar(&command.flags.literal, "literal", "", "print the blocks applied to CHUNK_REFERENCE literally")
	flag.StringVar(&command.flags.literal, "l", "", "print the blocks applied to CHUNK_REFERENCE literally")
	flag.StringVar(&command.flags.effective, "effective", "", "print the effective blocks of CHUNK_REFERENCE")
	flag.StringVar(&command.flags.effective, "e", "", "print the effective blocks of CHUNK_REFERENCE")
	flag.StringVar(&command.flags.depend, "depend", "", "list all blocks that depend on CHUNK_REFERENCE")
	flag.StringVar(&command.flags.depend, "d", "", "list all blocks that depend on CHUNK_REFERENCE")
	flag.StringVar(&command.flags.reference, "reference", "", "list all blocks that refer to ITEM")
	flag.StringVar(&command.flags.reference, "r", "", "list all blocks that refer to ITEM")
	flag.StringVar(&command.flags.print, "print", "", "evaluate EXPRESSION, and print it")
	flag.StringVar(&command.flags.print, "p", "", "evaluate EXPRESSION, and print it")
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), "Usage: litar [-a] [-f ARCHIVE] [-l CHUNK_REFERENCE] [-e CHUNK_REFERENCE] [-d CHUNK_REFERENCE] [-r ITEM] [-p EXPRESSION]\n\n-h, --help: print the usage\n-a, --agent: open an MCP server to handle the requests from agents\n-f, --file ARCHIVE: specify the archive file, find the composition and the node corresponding\n-l, --literal CHUNK_REFERENCE: print the blocks applied to CHUNK_REFERENCE literally\n-e, --effective CHUNK_REFERENCE: print the effective blocks of CHUNK_REFERENCE\n-d, --depend CHUNK_REFERENCE: list all blocks that depend on CHUNK_REFERENCE\n-r, --reference ITEM: list all blocks that refer to ITEM\n-p, --print EXPRESSION: evaluate EXPRESSION, and print it\n")
	}
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.Usage = func() {}
	if err := flag.CommandLine.Parse(os.Args[1:]); err == flag.ErrHelp {
		flag.CommandLine.SetOutput(os.Stdout)
		flag.Usage()
		os.Exit(0)
	} else if err != nil {
		flag.Usage()
		os.Exit(2)
	}
	if len(flag.Args()) != 0 {
		flag.Usage()
		os.Exit(1)
	}

	os.Exit(litar.Main(litar.Options{
		Agent:     command.flags.agent,
		File:      command.flags.file,
		Literal:   command.flags.literal,
		Effective: command.flags.effective,
		Depend:    command.flags.depend,
		Reference: command.flags.reference,
		Print:     command.flags.print,
	}))
}
