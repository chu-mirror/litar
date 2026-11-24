//go:build linux

package litar

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

func mountFUSE(dir string, root *dirNode, opts *fs.Options, tree *vdir, overlay string) (string, *fuse.Server, bool, error) {
	opts.DirectMount = true
	server, err := fs.Mount(dir, root, opts)
	direct := err == nil
	if err != nil {
		os.RemoveAll(dir)
		dir, err = os.MkdirTemp("", "litar-mnt-*")
		if err != nil {
			os.RemoveAll(overlay)
			return "", nil, false, fmtMount(err)
		}
		opts.DirectMount = false
		root = &dirNode{rel: "", spec: tree, overlay: overlay}
		server, err = fs.Mount(dir, root, opts)
	}
	if err != nil {
		os.RemoveAll(dir)
		os.RemoveAll(overlay)
		return "", nil, false, fmtMount(err)
	}
	return dir, server, direct, nil
}

func detach(dir string, direct bool) {
	if direct && syscall.Unmount(dir, syscall.MNT_DETACH) == nil {
		return
	}
	for _, prog := range []string{"fusermount3", "fusermount"} {
		if exec.Command(prog, "-u", "-z", dir).Run() == nil {
			return
		}
	}
}

func fuseAvailable() bool {
	_, err := os.Stat("/dev/fuse")
	return err == nil
}

func countLitarMounts() (int, bool) {
	data, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return 0, false
	}
	return strings.Count(string(data), "fuse.litar"), true
}
