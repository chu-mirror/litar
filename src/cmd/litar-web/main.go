// Command litar-web serves a browser interface for litar compositions.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var command struct {
	flags struct {
		addr  string
		litar string
		file  string
	}
}

//go:embed index.html
var indexHTML []byte

var readOnlyTools = map[string]bool{
	"import_composition":  true,
	"evaluate_expression": true,
}

func main() {
	flag.StringVar(&command.flags.addr, "addr", "127.0.0.1:8080", "address to listen on")
	flag.StringVar(&command.flags.litar, "litar", "litar", "litar binary that serves the compositions")
	flag.StringVar(&command.flags.file, "file", "", "archive whose composition is imported at start")
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), "Usage: litar-web [--addr ADDR] [--litar LITAR] [--file FILE]\n\n-h, --help: print the usage\n--addr ADDR: address to listen on (default \"127.0.0.1:8080\")\n--litar LITAR: litar binary that serves the compositions (default \"litar\")\n--file FILE: archive whose composition is imported at start\n")
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

	session, err := connect(command.flags.litar, command.flags.file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "litar-web:", err)
		os.Exit(1)
	}
	defer session.Close()
	http.HandleFunc("GET /{$}", serveIndex)
	http.HandleFunc("GET /api/resource", resourceHandler(session))
	http.HandleFunc("POST /api/tool", toolHandler(session))
	fmt.Fprintf(os.Stderr, "litar-web: serving on http://%s/\n", command.flags.addr)
	if err := http.ListenAndServe(command.flags.addr, nil); err != nil {
		fmt.Fprintln(os.Stderr, "litar-web:", err)
		os.Exit(1)
	}
}
func connect(litar, file string) (*mcp.ClientSession, error) {
	args := []string{"-a"}
	if file != "" {
		args = append(args, "-f", file)
	}
	cmd := exec.Command(litar, args...)
	cmd.Stderr = os.Stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "litar-web", Version: "0.1.0"}, nil)
	return client.Connect(context.Background(), &mcp.CommandTransport{Command: cmd}, nil)
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func resourceHandler(session *mcp.ClientSession) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uri := r.URL.Query().Get("uri")
		res, err := session.ReadResource(r.Context(), &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(res.Contents) == 0 {
			http.Error(w, "resource '"+uri+"' is empty", http.StatusNotFound)
			return
		}
		c := res.Contents[0]
		mime := c.MIMEType
		if mime == "" {
			mime = "text/plain"
		}
		if c.Blob != nil {
			w.Header().Set("Content-Type", mime)
			w.Write(c.Blob)
			return
		}
		w.Header().Set("Content-Type", mime+"; charset=utf-8")
		io.WriteString(w, c.Text)
	}
}

func toolHandler(session *mcp.ClientSession) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "the request must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !readOnlyTools[call.Name] {
			http.Error(w, "tool '"+call.Name+"' is not available", http.StatusForbidden)
			return
		}
		res, err := session.CallTool(r.Context(), &mcp.CallToolParams{Name: call.Name, Arguments: call.Arguments})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		var text strings.Builder
		for _, c := range res.Content {
			if t, ok := c.(*mcp.TextContent); ok {
				text.WriteString(t.Text)
			}
		}
		if res.IsError {
			http.Error(w, text.String(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, text.String())
	}
}
