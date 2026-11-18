package litar

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var filterCacheDir string

type countingWriter struct {
	w io.Writer
	n int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += n
	return n, err
}

func cacheDirFromEnv() string {
	dir, ok := os.LookupEnv("LITAR_CACHE")
	if !ok {
		base, err := os.UserCacheDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(base, "litar")
	}
	if dir == "" || dir == "off" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	return abs
}

func filterKey(script, input string, files []viewFile) (string, bool) {
	if filterCacheDir == "" {
		return "", false
	}
	h := sha256.New()
	fmt.Fprintf(h, "litar filter 1\n%d:%s%d:%s", len(script), script, len(input), input)
	for _, f := range files {
		if !f.archive {
			return "", false
		}
		fmt.Fprintf(h, "%q %t %d:", f.path, f.executable, len(f.body))
		h.Write(f.body)
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

func cachedOutputPath(key string) string {
	return filepath.Join(filterCacheDir, "filters", key[:2], key)
}

// readCachedOutput returns the output kept for key, if there is one.
func readCachedOutput(key string) (string, bool) {
	data, err := os.ReadFile(cachedOutputPath(key))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// writeCachedOutput keeps out as the output for key.
func writeCachedOutput(key, out string) {
	path := cachedOutputPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(path), key+".*")
	if err != nil {
		return
	}
	_, err = f.WriteString(out)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil || os.Rename(f.Name(), path) != nil {
		os.Remove(f.Name())
	}
}

func filterEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "LITAR_CACHE=") {
			env = append(env, kv)
		}
	}
	if filterCacheDir != "" {
		env = append(env, "LITAR_CACHE="+filterCacheDir)
	}
	return env
}
