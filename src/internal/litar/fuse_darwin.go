//go:build darwin

package litar

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

const mntForce = 0x80000

func mountFUSE(dir string, root *dirNode, opts *fs.Options, _ *vdir, overlay string) (string, *fuse.Server, bool, error) {
	opts.Options = append(opts.Options, "nobrowse", "noappledouble", "noapplexattr")
	server, err := fs.Mount(dir, root, opts)
	if err != nil {
		os.RemoveAll(dir)
		os.RemoveAll(overlay)
		return "", nil, false, fmtMount(err)
	}
	return dir, server, false, nil
}

func detach(dir string, _ bool) {
	if syscall.Unmount(dir, mntForce) == nil {
		return
	}
	if exec.Command("umount", "-f", dir).Run() == nil {
		return
	}
	_ = exec.Command("diskutil", "unmount", "force", dir).Run()
}

func fuseAvailable() bool {
	for _, p := range []string{
		"/Library/Filesystems/macfuse.fs/Contents/Resources/mount_macfuse",
		"/Library/Filesystems/osxfuse.fs/Contents/Resources/mount_osxfuse",
	} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func countLitarMounts() (int, bool) {
	out, err := exec.Command("mount").Output()
	if err != nil {
		return 0, false
	}
	return strings.Count(string(out), "litar on "), true
}
