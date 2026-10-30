package litar

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type ventry struct {
	file *viewFile
	dir  *vdir
}

type vdir struct {
	kids map[string]*ventry
}

func newVdir() *vdir {
	return &vdir{kids: map[string]*ventry{}}
}

func buildVTree(files []viewFile) (*vdir, error) {
	root := newVdir()
	for i := range files {
		f := &files[i]
		parts := stringsSplit(f.path)
		dir := root
		for i, part := range parts {
			last := i == len(parts)-1
			cur := dir.kids[part]
			if last {
				if cur != nil && cur.dir != nil {
					return nil, errPath(f.path)
				}
				if cur != nil && cur.file != nil {
					return nil, errPath(f.path)
				}
				dir.kids[part] = &ventry{file: f}
				continue
			}
			if cur != nil && cur.file != nil {
				return nil, errPath(f.path)
			}
			if cur == nil {
				cur = &ventry{dir: newVdir()}
				dir.kids[part] = cur
			}
			dir = cur.dir
		}
	}
	return root, nil
}

func errPath(p string) error {
	return &pathUseError{p}
}

type pathUseError struct{ path string }

func (e *pathUseError) Error() string {
	return "invalid file path '" + e.path + "'"
}

func stringsSplit(p string) []string {
	return splitSlash(p)
}

func splitSlash(p string) []string {
	var out []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			out = append(out, p[start:i])
			start = i + 1
		}
	}
	out = append(out, p[start:])
	return out
}

type mount struct {
	dir    string
	server *fuse.Server
	extra  []string
}

func (m *mount) close() {
	if m == nil {
		return
	}
	if m.server != nil {
		_ = m.server.Unmount()
		m.server.Wait()
	}
	if m.dir != "" {
		_ = os.RemoveAll(m.dir)
	}
	for _, p := range m.extra {
		_ = os.RemoveAll(p)
	}
}

func mountView(ev *Evaluator, files []viewFile) (*mount, error) {
	tree, err := buildVTree(files)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "litar-mnt-*")
	if err != nil {
		return nil, fmtMount(err)
	}
	overlay, err := os.MkdirTemp("", "litar-ovl-*")
	if err != nil {
		os.RemoveAll(dir)
		return nil, fmtMount(err)
	}
	root := &dirNode{ev: ev, rel: "", spec: tree, overlay: overlay}
	zero := time.Duration(0)
	opts := &fs.Options{
		EntryTimeout:    &zero,
		AttrTimeout:     &zero,
		NullPermissions: true,
	}
	opts.FsName = "litar"
	opts.Name = "litar"
	opts.DirectMount = true
	server, err := fs.Mount(dir, root, opts)
	if err != nil {
		os.RemoveAll(dir)
		dir, err = os.MkdirTemp("", "litar-mnt-*")
		if err != nil {
			os.RemoveAll(overlay)
			return nil, fmtMount(err)
		}
		opts.DirectMount = false
		root = &dirNode{ev: ev, rel: "", spec: tree, overlay: overlay}
		server, err = fs.Mount(dir, root, opts)
	}
	if err != nil {
		os.RemoveAll(dir)
		os.RemoveAll(overlay)
		return nil, fmtMount(err)
	}
	return &mount{dir: dir, server: server, extra: []string{overlay}}, nil
}

func fmtMount(err error) error {
	return &mountError{err}
}

type mountError struct{ err error }

func (e *mountError) Error() string {
	return "could not mount the filter view: " + e.err.Error()
}

type dirNode struct {
	fs.Inode
	ev      *Evaluator
	rel     string
	spec    *vdir
	overlay string
}

type fileNode struct {
	fs.Inode
	ev   *Evaluator
	file *viewFile
}

type overlayNode struct {
	fs.Inode
	host string
	dir  bool
}

func (d *dirNode) hostDir() string {
	if d.rel == "" {
		return d.overlay
	}
	return filepath.Join(d.overlay, d.rel)
}

func (d *dirNode) specChild(name string) *ventry {
	if d.spec == nil {
		return nil
	}
	return d.spec.kids[name]
}

func (d *dirNode) childRel(name string) string {
	if d.rel == "" {
		return name
	}
	return d.rel + "/" + name
}

func modeOf(f *viewFile) uint32 {
	perm := uint32(0o444)
	if f.executable {
		perm = 0o555
	}
	return syscall.S_IFREG | perm
}

func (d *dirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if ch := d.specChild(name); ch != nil {
		if ch.file != nil {
			mode := modeOf(ch.file)
			out.Mode = mode
			node := &fileNode{ev: d.ev, file: ch.file}
			return d.NewInode(ctx, node, fs.StableAttr{Mode: syscall.S_IFREG}), 0
		}
		out.Mode = syscall.S_IFDIR | 0o755
		node := &dirNode{ev: d.ev, rel: d.childRel(name), spec: ch.dir, overlay: d.overlay}
		return d.NewInode(ctx, node, fs.StableAttr{Mode: syscall.S_IFDIR}), 0
	}
	host := filepath.Join(d.hostDir(), name)
	st, err := os.Lstat(host)
	if err != nil {
		return nil, syscall.ENOENT
	}
	if st.IsDir() {
		out.Mode = syscall.S_IFDIR | 0o755
		node := &dirNode{ev: d.ev, rel: d.childRel(name), overlay: d.overlay}
		return d.NewInode(ctx, node, fs.StableAttr{Mode: syscall.S_IFDIR}), 0
	}
	out.Mode = uint32(st.Mode())
	out.Size = uint64(st.Size())
	node := &overlayNode{host: host}
	return d.NewInode(ctx, node, fs.StableAttr{Mode: syscall.S_IFREG}), 0
}

func (d *dirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	names := map[string]uint32{}
	if d.spec != nil {
		for name, ch := range d.spec.kids {
			if ch.file != nil {
				names[name] = syscall.S_IFREG
			} else {
				names[name] = syscall.S_IFDIR
			}
		}
	}
	entries, err := os.ReadDir(d.hostDir())
	if err == nil {
		for _, ent := range entries {
			if _, ok := names[ent.Name()]; ok {
				continue
			}
			if ent.IsDir() {
				names[ent.Name()] = syscall.S_IFDIR
			} else {
				names[ent.Name()] = syscall.S_IFREG
			}
		}
	}
	list := []fuse.DirEntry{
		{Name: ".", Mode: syscall.S_IFDIR},
		{Name: "..", Mode: syscall.S_IFDIR},
	}
	order := make([]string, 0, len(names))
	for name := range names {
		order = append(order, name)
	}
	sort.Strings(order)
	for _, name := range order {
		list = append(list, fuse.DirEntry{Name: name, Mode: names[name]})
	}
	return fs.NewListDirStream(list), 0
}

func (d *dirNode) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFDIR | 0o755
	return 0
}

func (d *dirNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	if d.specChild(name) != nil {
		return nil, nil, 0, syscall.EEXIST
	}
	host := filepath.Join(d.hostDir(), name)
	if err := os.MkdirAll(d.hostDir(), 0o755); err != nil {
		return nil, nil, 0, syscall.EIO
	}
	fh, err := os.OpenFile(host, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, nil, 0, syscall.EEXIST
		}
		return nil, nil, 0, syscall.EIO
	}
	out.Mode = syscall.S_IFREG | 0o644
	node := &overlayNode{host: host}
	ino := d.NewInode(ctx, node, fs.StableAttr{Mode: syscall.S_IFREG})
	return ino, fs.NewLoopbackFileFromOS(fh), 0, 0
}

func (d *dirNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if d.specChild(name) != nil {
		return nil, syscall.EEXIST
	}
	host := filepath.Join(d.hostDir(), name)
	if err := os.MkdirAll(host, 0o755); err != nil {
		return nil, syscall.EIO
	}
	out.Mode = syscall.S_IFDIR | 0o755
	node := &dirNode{ev: d.ev, rel: d.childRel(name), overlay: d.overlay}
	return d.NewInode(ctx, node, fs.StableAttr{Mode: syscall.S_IFDIR}), 0
}

func (d *dirNode) Unlink(ctx context.Context, name string) syscall.Errno {
	if d.specChild(name) != nil {
		return syscall.EROFS
	}
	host := filepath.Join(d.hostDir(), name)
	if err := os.Remove(host); err != nil {
		if os.IsNotExist(err) {
			return syscall.ENOENT
		}
		return syscall.EIO
	}
	return 0
}

func (d *dirNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	if ch := d.specChild(name); ch != nil {
		return syscall.EROFS
	}
	host := filepath.Join(d.hostDir(), name)
	if err := os.Remove(host); err != nil {
		if os.IsNotExist(err) {
			return syscall.ENOENT
		}
		return syscall.EIO
	}
	return 0
}

func (f *fileNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = modeOf(f.file)
	// Mapped host files report their real size. Tools such as clang-format
	// trust st_size and skip a file that looks empty. Archive files stay
	// unevaluated here; reads use direct I/O.
	if f.file != nil && !f.file.archive && f.file.origin != "" {
		st, err := os.Stat(f.file.origin)
		if err == nil {
			out.Size = uint64(st.Size())
		}
	}
	return 0
}

func (f *fileNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	acc := flags & syscall.O_ACCMODE
	if acc == syscall.O_WRONLY || acc == syscall.O_RDWR || flags&syscall.O_TRUNC != 0 || flags&syscall.O_APPEND != 0 {
		return nil, 0, syscall.EROFS
	}
	return &archiveFH{ev: f.ev, file: f.file}, fuse.FOPEN_DIRECT_IO, 0
}

func (f *fileNode) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	return syscall.EROFS
}

func (f *fileNode) Unlink(ctx context.Context, name string) syscall.Errno {
	return syscall.EROFS
}

type archiveFH struct {
	ev   *Evaluator
	file *viewFile
}

func (h *archiveFH) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	var (
		data []byte
		err  error
	)
	if h.file.archive {
		data, err = h.ev.readChunk(Name(h.file.path))
	} else {
		data, err = os.ReadFile(h.file.origin)
		if err != nil {
			h.ev.noteErr(err)
		}
	}
	if err != nil {
		return nil, syscall.EIO
	}
	return fuse.ReadResultData(window(data, dest, off)), 0
}

func window(data, dest []byte, off int64) []byte {
	if off < 0 || off >= int64(len(data)) {
		return nil
	}
	data = data[off:]
	if len(dest) < len(data) {
		data = data[:len(dest)]
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out
}

func (o *overlayNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	st, err := os.Stat(o.host)
	if err != nil {
		return syscall.ENOENT
	}
	out.Mode = uint32(st.Mode())
	out.Size = uint64(st.Size())
	return 0
}

func (o *overlayNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	// The kernel already applied the open mode. Read-write lets a
	// later read of the same descriptor see an earlier write.
	fh, err := os.OpenFile(o.host, os.O_RDWR, 0)
	if err != nil {
		return nil, 0, syscall.EIO
	}
	return fs.NewLoopbackFileFromOS(fh), 0, 0
}

var (
	_ fs.NodeLookuper  = (*dirNode)(nil)
	_ fs.NodeReaddirer = (*dirNode)(nil)
	_ fs.NodeGetattrer = (*dirNode)(nil)
	_ fs.NodeCreater   = (*dirNode)(nil)
	_ fs.NodeMkdirer   = (*dirNode)(nil)
	_ fs.NodeUnlinker  = (*dirNode)(nil)
	_ fs.NodeRmdirer   = (*dirNode)(nil)
	_ fs.NodeGetattrer = (*fileNode)(nil)
	_ fs.NodeOpener    = (*fileNode)(nil)
	_ fs.NodeSetattrer = (*fileNode)(nil)
	_ fs.FileReader    = (*archiveFH)(nil)
	_ fs.NodeGetattrer = (*overlayNode)(nil)
	_ fs.NodeOpener    = (*overlayNode)(nil)
)
