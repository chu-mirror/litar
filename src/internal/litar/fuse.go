package litar

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

const (
	fuseDaemonEnv    = "LITAR_FUSE_DAEMON_DIR"
	fuseProtoVersion = 1
)

var (
	liveMu      sync.Mutex
	liveClients = map[*fuseClient]struct{}{}
)

var fixedStatTime = time.Unix(0, 0)

var (
	_ fs.NodeLookuper  = (*dirNode)(nil)
	_ fs.NodeReaddirer = (*dirNode)(nil)
	_ fs.NodeGetattrer = (*dirNode)(nil)
	_ fs.NodeSetattrer = (*dirNode)(nil)
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

type ventry struct {
	file *viewFile
	dir  *vdir
	// host is the origin of a mapped directory, and ino its inode number;
	// readOnly is set when the mapping belongs to the ground.
	host     string
	ino      uint64
	readOnly bool
}

type vdir struct {
	kids map[string]*ventry
	ino  uint64
}

type pathUseError struct{ path string }

type mount struct {
	dir    string
	slot   string
	client *fuseClient
}

type mountError struct{ err error }

type dirNode struct {
	fs.Inode
	rel     string
	spec    *vdir
	overlay string
}

type fileNode struct {
	fs.Inode
	file *viewFile
}

type overlayNode struct {
	fs.Inode
	host string
	dir  bool
}

type archiveFH struct {
	file *viewFile
}

type readOnlyNode struct {
	*fs.LoopbackNode
}

type rootNode struct {
	fs.Inode
	mu    sync.RWMutex
	slots map[string]*viewSlot
}

type viewSlot struct {
	tree    *vdir
	overlay string
}

type fuseReq struct {
	Op       string         `json:"op"`
	Version  int            `json:"version,omitempty"`
	Slot     string         `json:"slot,omitempty"`
	MtimeDir string         `json:"mtime_dir,omitempty"`
	Files    []wireViewFile `json:"files,omitempty"`
}

type wireViewFile struct {
	Path       string `json:"path"`
	Executable bool   `json:"executable,omitempty"`
	Origin     string `json:"origin,omitempty"`
	Archive    bool   `json:"archive,omitempty"`
	Dir        bool   `json:"dir,omitempty"`
	ReadOnly   bool   `json:"read_only,omitempty"`
	Body       []byte `json:"body,omitempty"`
}

type fuseResp struct {
	OK    bool   `json:"ok"`
	Dir   string `json:"dir,omitempty"`
	Error string `json:"error,omitempty"`
}

type mtimeRecord struct {
	Hash    string `json:"hash"`
	MtimeNS int64  `json:"mtime_ns"`
}

type fuseClient struct {
	mu      sync.Mutex
	conn    net.Conn
	enc     *json.Encoder
	dec     *json.Decoder
	baseDir string
	closed  bool
}

type slotState struct {
	owner *daemonClient
	slot  *viewSlot
	done  chan struct{}
}

type daemonClient struct {
	mu     sync.Mutex
	slots  map[string]struct{}
	closed chan struct{}
}

type fuseDaemon struct {
	baseDir  string
	mntDir   string
	sockPath string
	lockPath string
	root     *rootNode
	server   *fuse.Server
	direct   bool
	ln       net.Listener
	done     chan struct{}
	exitOnce sync.Once

	mu            sync.Mutex
	clients       int
	everConnected bool
	closing       bool
	nextIno       uint64
	slots         map[string]*slotState
}

func (e *pathUseError) Error() string {
	return "invalid file path '" + e.path + "'"
}

func (m *mount) close() {
	if m == nil || m.client == nil || m.slot == "" {
		return
	}
	_, _ = m.client.roundTrip(fuseReq{Op: "unmount", Slot: m.slot})
	m.slot = ""
}

func (e *mountError) Error() string {
	return "could not mount the filter view: " + e.err.Error()
}

func (d *dirNode) lookupHost(ctx context.Context, ch *ventry, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	var st syscall.Stat_t
	if err := syscall.Stat(ch.host, &st); err != nil {
		return nil, fs.ToErrno(err)
	}
	out.Attr.FromStat(&st)
	out.Attr.Ino = ch.ino
	return d.NewInode(ctx, hostNode(ch.host, ch.ino, ch.readOnly), fs.StableAttr{Mode: syscall.S_IFDIR, Ino: ch.ino}), 0
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

func (d *dirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if ch := d.specChild(name); ch != nil {
		if ch.host != "" {
			return d.lookupHost(ctx, ch, out)
		}
		if ch.file != nil {
			setFileAttr(ch.file, &out.Attr)
			node := &fileNode{file: ch.file}
			return d.NewInode(ctx, node, fs.StableAttr{Mode: syscall.S_IFREG, Ino: ch.file.ino}), 0
		}
		setDirAttr(ch.dir, &out.Attr)
		node := &dirNode{rel: d.childRel(name), spec: ch.dir, overlay: d.overlay}
		return d.NewInode(ctx, node, fs.StableAttr{Mode: syscall.S_IFDIR, Ino: ch.dir.ino}), 0
	}
	host := filepath.Join(d.hostDir(), name)
	st, err := os.Lstat(host)
	if err != nil {
		return nil, syscall.ENOENT
	}
	if st.IsDir() {
		out.Mode = syscall.S_IFDIR | 0o755
		node := &dirNode{rel: d.childRel(name), overlay: d.overlay}
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
	if d.spec == nil {
		out.Mode = syscall.S_IFDIR | 0o755
		return 0
	}
	setDirAttr(d.spec, &out.Attr)
	return 0
}

func (d *dirNode) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if d.spec == nil {
		return syscall.ENOTSUP
	}
	setDirAttr(d.spec, &out.Attr)
	return syscall.EROFS
}

func (d *dirNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	if d.specChild(name) != nil {
		return nil, nil, 0, syscall.EEXIST
	}
	host := filepath.Join(d.hostDir(), name)
	var fh *os.File
	if err := os.MkdirAll(d.hostDir(), 0o755); err != nil {
		return nil, nil, 0, syscall.EIO
	}
	var err error
	fh, err = os.OpenFile(host, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0o644)
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
	node := &dirNode{rel: d.childRel(name), overlay: d.overlay}
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
	// The file is tangled before the process starts. Tools such as
	// clang-format trust st_size and skip a file that looks empty.
	// The rest of the stat stays at the values fixed for this invocation.
	setFileAttr(f.file, &out.Attr)
	return 0
}

func (f *fileNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	acc := flags & syscall.O_ACCMODE
	if acc == syscall.O_WRONLY || acc == syscall.O_RDWR || flags&syscall.O_TRUNC != 0 || flags&syscall.O_APPEND != 0 {
		return nil, 0, syscall.EROFS
	}
	return &archiveFH{file: f.file}, 0, 0
}

func (f *fileNode) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	setFileAttr(f.file, &out.Attr)
	return syscall.EROFS
}

func (f *fileNode) Unlink(ctx context.Context, name string) syscall.Errno {
	return syscall.EROFS
}

func (h *archiveFH) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if h.file == nil {
		return nil, syscall.EIO
	}
	return fuse.ReadResultData(window(h.file.body, dest, off)), 0
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

func (n *readOnlyNode) WrapChild(_ context.Context, ops fs.InodeEmbedder) fs.InodeEmbedder {
	return &readOnlyNode{ops.(*fs.LoopbackNode)}
}

func (n *readOnlyNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	if flags&syscall.O_ACCMODE != syscall.O_RDONLY || flags&syscall.O_TRUNC != 0 {
		return nil, 0, syscall.EROFS
	}
	return n.LoopbackNode.Open(ctx, flags)
}

func (n *readOnlyNode) Create(context.Context, string, uint32, uint32, *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	return nil, nil, 0, syscall.EROFS
}

func (n *readOnlyNode) Setattr(context.Context, fs.FileHandle, *fuse.SetAttrIn, *fuse.AttrOut) syscall.Errno {
	return syscall.EROFS
}

func (n *readOnlyNode) Mkdir(context.Context, string, uint32, *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	return nil, syscall.EROFS
}

func (n *readOnlyNode) Mknod(context.Context, string, uint32, uint32, *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	return nil, syscall.EROFS
}

func (n *readOnlyNode) Symlink(context.Context, string, string, *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	return nil, syscall.EROFS
}

func (n *readOnlyNode) Link(context.Context, fs.InodeEmbedder, string, *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	return nil, syscall.EROFS
}

func (n *readOnlyNode) Unlink(context.Context, string) syscall.Errno { return syscall.EROFS }

func (n *readOnlyNode) Rmdir(context.Context, string) syscall.Errno { return syscall.EROFS }

func (n *readOnlyNode) Rename(context.Context, string, fs.InodeEmbedder, string, uint32) syscall.Errno {
	return syscall.EROFS
}

func (n *readOnlyNode) Setxattr(context.Context, string, []byte, uint32) syscall.Errno {
	return syscall.EROFS
}

func (n *readOnlyNode) Removexattr(context.Context, string) syscall.Errno { return syscall.EROFS }

func (r *rootNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	r.mu.RLock()
	slot := r.slots[name]
	r.mu.RUnlock()
	if slot == nil {
		return nil, syscall.ENOENT
	}
	setDirAttr(slot.tree, &out.Attr)
	node := &dirNode{rel: "", spec: slot.tree, overlay: slot.overlay}
	return r.NewInode(ctx, node, fs.StableAttr{Mode: syscall.S_IFDIR, Ino: slot.tree.ino}), 0
}

func (r *rootNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	r.mu.RLock()
	names := make([]string, 0, len(r.slots))
	for name := range r.slots {
		names = append(names, name)
	}
	r.mu.RUnlock()
	sort.Strings(names)
	list := make([]fuse.DirEntry, 0, len(names)+2)
	list = append(list,
		fuse.DirEntry{Name: ".", Mode: syscall.S_IFDIR},
		fuse.DirEntry{Name: "..", Mode: syscall.S_IFDIR},
	)
	for _, name := range names {
		list = append(list, fuse.DirEntry{Name: name, Mode: syscall.S_IFDIR})
	}
	return fs.NewListDirStream(list), 0
}

func (r *rootNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	r.mu.RLock()
	n := len(r.slots)
	r.mu.RUnlock()
	out.Mode = syscall.S_IFDIR | 0o755
	out.Nlink = uint32(2 + n)
	out.SetTimes(&fixedStatTime, &fixedStatTime, &fixedStatTime)
	return 0
}

func (r *rootNode) setSlot(name string, s *viewSlot) {
	r.mu.Lock()
	r.slots[name] = s
	r.mu.Unlock()
}

func (r *rootNode) delSlot(name string) {
	r.mu.Lock()
	delete(r.slots, name)
	r.mu.Unlock()
	r.RmChild(name)
}

func (c *fuseClient) roundTrip(req fuseReq) (fuseResp, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fuseResp{}, errors.New("fuse client closed")
	}
	if err := c.enc.Encode(req); err != nil {
		return fuseResp{}, err
	}
	var resp fuseResp
	if err := c.dec.Decode(&resp); err != nil {
		return fuseResp{}, err
	}
	return resp, nil
}

func (c *fuseClient) close() {
	if c == nil {
		return
	}
	liveMu.Lock()
	delete(liveClients, c)
	liveMu.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	_ = c.conn.SetDeadline(time.Now().Add(5 * time.Second))
	_ = c.enc.Encode(fuseReq{Op: "bye"})
	var resp fuseResp
	_ = c.dec.Decode(&resp)
	_ = c.conn.Close()
	c.mu.Unlock()
}

func (e *Evaluator) mountView(slot string, files []viewFile) (*mount, error) {
	if _, _, err := buildVTree(files, 2); err != nil {
		return nil, err
	}
	fc, err := e.fuseSession()
	if err != nil {
		return nil, err
	}
	wire := make([]wireViewFile, len(files))
	for i, f := range files {
		wire[i] = wireViewFile{
			Path:       f.path,
			Executable: f.executable,
			Origin:     f.origin,
			Archive:    f.archive,
			Dir:        f.dir,
			ReadOnly:   f.readOnly,
			Body:       f.body,
		}
	}
	resp, err := fc.roundTrip(fuseReq{
		Op:       "mount",
		Slot:     slot,
		MtimeDir: mtimeStateDir(fc.baseDir),
		Files:    wire,
	})
	if err != nil {
		return nil, fmtMount(err)
	}
	if !resp.OK {
		return nil, errors.New(resp.Error)
	}
	return &mount{dir: resp.Dir, slot: slot, client: fc}, nil
}

func (d *fuseDaemon) handleConn(conn net.Conn) {
	defer conn.Close()
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)
	client := &daemonClient{
		slots:  map[string]struct{}{},
		closed: make(chan struct{}),
	}
	reqCh := make(chan fuseReq, 1)
	go func() {
		defer close(client.closed)
		defer close(reqCh)
		for {
			var req fuseReq
			if err := dec.Decode(&req); err != nil {
				return
			}
			reqCh <- req
		}
	}()
	registered := false
	defer func() {
		d.releaseClientSlots(client)
		if registered {
			d.disconnectClient(enc, conn, false)
		}
	}()
	for req := range reqCh {
		switch req.Op {
		case "hello":
			if req.Version != fuseProtoVersion {
				_ = enc.Encode(fuseResp{Error: "incompatible FUSE daemon protocol"})
				return
			}
			d.mu.Lock()
			if d.closing {
				d.mu.Unlock()
				_ = enc.Encode(fuseResp{Error: "daemon shutting down"})
				return
			}
			if !registered {
				d.clients++
				d.everConnected = true
				registered = true
			}
			d.mu.Unlock()
			_ = enc.Encode(fuseResp{OK: true})
		case "mount":
			if !registered {
				_ = enc.Encode(fuseResp{Error: "not registered"})
				return
			}
			resp := d.handleMount(client, req)
			_ = enc.Encode(resp)
		case "unmount":
			d.releaseSlot(client, req.Slot)
			_ = enc.Encode(fuseResp{OK: true})
		case "bye":
			d.releaseClientSlots(client)
			if registered {
				registered = false
				d.disconnectClient(enc, conn, true)
			} else {
				_ = enc.Encode(fuseResp{OK: true})
			}
			return
		default:
			_ = enc.Encode(fuseResp{Error: "unknown op"})
		}
	}
}

func (d *fuseDaemon) handleMount(client *daemonClient, req fuseReq) fuseResp {
	if req.Slot == "" {
		return fuseResp{Error: "empty view slot"}
	}
	var st *slotState
	for {
		d.mu.Lock()
		if d.closing {
			d.mu.Unlock()
			return fuseResp{Error: "daemon shutting down"}
		}
		cur := d.slots[req.Slot]
		if cur == nil {
			st = &slotState{owner: client, done: make(chan struct{})}
			d.slots[req.Slot] = st
			d.mu.Unlock()
			break
		}
		waitCh := cur.done
		d.mu.Unlock()
		select {
		case <-waitCh:
		case <-client.closed:
			return fuseResp{Error: "client closed"}
		}
	}
	abort := func(err error) fuseResp {
		d.mu.Lock()
		if d.slots[req.Slot] == st {
			delete(d.slots, req.Slot)
			close(st.done)
		}
		d.mu.Unlock()
		return fuseResp{Error: err.Error()}
	}
	files := make([]viewFile, len(req.Files))
	for i, wf := range req.Files {
		files[i] = viewFile{
			path:       wf.Path,
			executable: wf.Executable,
			origin:     wf.Origin,
			archive:    wf.Archive,
			dir:        wf.Dir,
			readOnly:   wf.ReadOnly,
			body:       wf.Body,
		}
	}
	applyTrackedMtimes(req.MtimeDir, req.Slot, files)
	d.mu.Lock()
	startIno := d.nextIno
	tree, nextIno, err := buildVTree(files, startIno)
	if err == nil {
		d.nextIno = nextIno
	}
	d.mu.Unlock()
	if err != nil {
		return abort(err)
	}
	overlay, err := os.MkdirTemp("", "litar-ovl-*")
	if err != nil {
		return abort(fmtMount(err))
	}
	slot := &viewSlot{tree: tree, overlay: overlay}
	d.mu.Lock()
	if d.closing || d.slots[req.Slot] != st {
		d.mu.Unlock()
		_ = os.RemoveAll(overlay)
		return abort(errors.New("daemon shutting down"))
	}
	st.slot = slot
	d.mu.Unlock()
	client.mu.Lock()
	client.slots[req.Slot] = struct{}{}
	client.mu.Unlock()
	d.root.setSlot(req.Slot, slot)
	return fuseResp{OK: true, Dir: filepath.Join(d.mntDir, req.Slot)}
}

func (d *fuseDaemon) releaseSlot(client *daemonClient, name string) {
	client.mu.Lock()
	_, owned := client.slots[name]
	delete(client.slots, name)
	client.mu.Unlock()
	if !owned {
		return
	}
	d.root.delSlot(name)
	d.mu.Lock()
	st := d.slots[name]
	if st != nil && st.owner == client {
		delete(d.slots, name)
		close(st.done)
	}
	d.mu.Unlock()
	if st != nil && st.slot != nil && st.slot.overlay != "" {
		_ = os.RemoveAll(st.slot.overlay)
	}
}

func (d *fuseDaemon) releaseClientSlots(client *daemonClient) {
	client.mu.Lock()
	names := make([]string, 0, len(client.slots))
	for name := range client.slots {
		names = append(names, name)
	}
	client.mu.Unlock()
	for _, name := range names {
		d.releaseSlot(client, name)
	}
}

func (d *fuseDaemon) disconnectClient(enc *json.Encoder, conn net.Conn, replyBye bool) {
	d.mu.Lock()
	if d.clients > 1 {
		d.clients--
		d.mu.Unlock()
		if replyBye {
			_ = enc.Encode(fuseResp{OK: true})
		}
		return
	}
	d.mu.Unlock()

	unlock, err := lockFuseFile(d.lockPath)
	d.mu.Lock()
	if d.closing {
		d.mu.Unlock()
		if err == nil {
			unlock()
		}
		if replyBye {
			_ = enc.Encode(fuseResp{OK: true})
		}
		return
	}
	if d.clients > 1 {
		d.clients--
		d.mu.Unlock()
		if err == nil {
			unlock()
		}
		if replyBye {
			_ = enc.Encode(fuseResp{OK: true})
		}
		return
	}
	d.clients = 0
	d.closing = true
	d.mu.Unlock()

	_ = d.ln.Close()
	_ = os.Remove(d.sockPath)
	if d.server != nil {
		if d.server.Unmount() != nil {
			detach(d.mntDir, d.direct)
		}
	}
	if replyBye {
		_ = enc.Encode(fuseResp{OK: true})
	}
	_ = conn.Close()
	if err == nil {
		unlock()
	}
	d.exitOnce.Do(func() { close(d.done) })
}

func (d *fuseDaemon) forceShutdown() {
	unlock, err := lockFuseFile(d.lockPath)
	d.mu.Lock()
	if d.closing {
		d.mu.Unlock()
		if err == nil {
			unlock()
		}
		return
	}
	d.closing = true
	var overlays []string
	for name, st := range d.slots {
		if st.slot != nil && st.slot.overlay != "" {
			overlays = append(overlays, st.slot.overlay)
		}
		delete(d.slots, name)
		close(st.done)
	}
	d.mu.Unlock()
	_ = d.ln.Close()
	_ = os.Remove(d.sockPath)
	if d.server != nil {
		if d.server.Unmount() != nil {
			detach(d.mntDir, d.direct)
		}
	}
	if err == nil {
		unlock()
	}
	for _, o := range overlays {
		_ = os.RemoveAll(o)
	}
	d.exitOnce.Do(func() { close(d.done) })
}

func newVdir() *vdir {
	return &vdir{kids: map[string]*ventry{}}
}

func buildVTree(files []viewFile, startIno uint64) (*vdir, uint64, error) {
	root := newVdir()
	for i := range files {
		f := &files[i]
		parts := stringsSplit(f.path)
		dir := root
		for i, part := range parts {
			last := i == len(parts)-1
			cur := dir.kids[part]
			if last {
				if cur != nil {
					return nil, 0, errPath(f.path)
				}
				if f.dir {
					dir.kids[part] = &ventry{host: f.origin, readOnly: f.readOnly}
				} else {
					dir.kids[part] = &ventry{file: f}
				}
				continue
			}
			if cur != nil && cur.dir == nil {
				return nil, 0, errPath(f.path)
			}
			if cur == nil {
				cur = &ventry{dir: newVdir()}
				dir.kids[part] = cur
			}
			dir = cur.dir
		}
	}
	next := assignInos(root, startIno)
	return root, next, nil
}

func assignInos(root *vdir, start uint64) uint64 {
	root.ino = start
	next := start + 1
	var walk func(*vdir)
	walk = func(d *vdir) {
		names := make([]string, 0, len(d.kids))
		for name := range d.kids {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ch := d.kids[name]
			switch {
			case ch.file != nil:
				ch.file.ino = next
			case ch.dir != nil:
				ch.dir.ino = next
			default:
				ch.ino = next
			}
			next++
			if ch.dir != nil {
				walk(ch.dir)
			}
		}
	}
	walk(root)
	return next
}

func errPath(p string) error {
	return &pathUseError{p}
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

func DetachMounts() {
	liveMu.Lock()
	clients := make([]*fuseClient, 0, len(liveClients))
	for c := range liveClients {
		clients = append(clients, c)
	}
	liveMu.Unlock()
	for _, c := range clients {
		c.close()
	}
}

func fmtMount(err error) error {
	return &mountError{err}
}

func hostNode(dir string, ino uint64, readOnly bool) fs.InodeEmbedder {
	data := &fs.LoopbackRoot{Path: dir, Dev: ino}
	node := &fs.LoopbackNode{RootData: data}
	data.RootNode = node
	if readOnly {
		return &readOnlyNode{node}
	}
	return node
}

func modeOf(f *viewFile) uint32 {
	perm := uint32(0o444)
	if f != nil && f.executable {
		perm = 0o555
	}
	return syscall.S_IFREG | perm
}

func setFileAttr(f *viewFile, out *fuse.Attr) {
	out.Mode = modeOf(f)
	out.Nlink = 1
	t := fixedStatTime
	if f != nil && !f.mtime.IsZero() {
		t = f.mtime
	}
	out.SetTimes(&t, &t, &t)
	if f != nil {
		out.Size = uint64(len(f.body))
	}
}

func dirNlink(d *vdir) uint32 {
	n := uint32(2)
	if d == nil {
		return n
	}
	for _, ch := range d.kids {
		if ch.dir != nil {
			n++
		}
	}
	return n
}

func setDirAttr(d *vdir, out *fuse.Attr) {
	out.Mode = syscall.S_IFDIR | 0o755
	out.Nlink = dirNlink(d)
	out.SetTimes(&fixedStatTime, &fixedStatTime, &fixedStatTime)
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

func init() {
	if dir := os.Getenv(fuseDaemonEnv); dir != "" {
		_ = os.Unsetenv(fuseDaemonEnv)
		runFuseDaemon(dir)
		os.Exit(0)
	}
}

func fuseBaseDir() (string, error) {
	dir := os.Getenv("LITAR_FUSE_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), fmt.Sprintf("litar-%d", os.Getuid()))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmtMount(err)
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return dir, nil
}

func mtimeStateDir(baseDir string) string {
	if filterCacheDir != "" {
		return filepath.Join(filterCacheDir, "mtimes")
	}
	if base, err := os.UserCacheDir(); err == nil && base != "" {
		return filepath.Join(base, "litar", "mtimes")
	}
	return filepath.Join(baseDir, "mtimes")
}

func applyTrackedMtimes(mtimeDir, slot string, files []viewFile) {
	hasWritableDir := false
	for _, f := range files {
		if f.dir && !f.readOnly {
			hasWritableDir = true
			break
		}
	}
	if !hasWritableDir || mtimeDir == "" {
		return
	}
	statePath := filepath.Join(mtimeDir, slot+".json")
	prev := map[string]mtimeRecord{}
	if data, err := os.ReadFile(statePath); err == nil {
		_ = json.Unmarshal(data, &prev)
	}
	now := time.Now().UnixNano()
	next := map[string]mtimeRecord{}
	changed := false
	for i := range files {
		if files[i].dir {
			continue
		}
		h := sha256.New()
		fmt.Fprintf(h, "%t:", files[i].executable)
		h.Write(files[i].body)
		sum := hex.EncodeToString(h.Sum(nil))
		rec, ok := prev[files[i].path]
		if ok && rec.Hash == sum && rec.MtimeNS > 0 {
			files[i].mtime = time.Unix(0, rec.MtimeNS)
			next[files[i].path] = rec
		} else {
			ts := now
			if ok && ts <= rec.MtimeNS {
				ts = rec.MtimeNS + 1
			}
			files[i].mtime = time.Unix(0, ts)
			next[files[i].path] = mtimeRecord{Hash: sum, MtimeNS: ts}
			changed = true
		}
	}
	if len(next) != len(prev) {
		changed = true
	}
	if !changed {
		return
	}
	if err := os.MkdirAll(mtimeDir, 0o755); err != nil {
		return
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(mtimeDir, slot+".*")
	if err != nil {
		return
	}
	_, err = tmp.Write(raw)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil || os.Rename(tmp.Name(), statePath) != nil {
		_ = os.Remove(tmp.Name())
	}
}

func lockFuseFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func dialFuseDaemon(baseDir, sockPath string) (*fuseClient, error) {
	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		return nil, err
	}
	c := &fuseClient{
		conn:    conn,
		enc:     json.NewEncoder(conn),
		dec:     json.NewDecoder(conn),
		baseDir: baseDir,
	}
	resp, err := c.roundTrip(fuseReq{Op: "hello", Version: fuseProtoVersion})
	if err != nil || !resp.OK {
		_ = conn.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New(resp.Error)
	}
	liveMu.Lock()
	liveClients[c] = struct{}{}
	liveMu.Unlock()
	return c, nil
}

func spawnFuseDaemon(baseDir string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmtMount(err)
	}
	cmd := exec.Command(exe)
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, fuseDaemonEnv+"=") {
			env = append(env, kv)
		}
	}
	cmd.Env = append(env, fuseDaemonEnv+"="+baseDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmtMount(err)
	}
	if err := cmd.Start(); err != nil {
		return fmtMount(err)
	}
	scanner := bufio.NewScanner(stdout)
	var line string
	if scanner.Scan() {
		line = scanner.Text()
	}
	_ = stdout.Close()
	if line == "OK" {
		go func() { _ = cmd.Wait() }()
		return nil
	}
	_ = cmd.Wait()
	if msg, ok := strings.CutPrefix(line, "ERR:"); ok {
		return errors.New(msg)
	}
	return fmtMount(errors.New("shared FUSE daemon exited before ready"))
}

func connectFuseDaemon() (*fuseClient, error) {
	baseDir, err := fuseBaseDir()
	if err != nil {
		return nil, err
	}
	sockPath := fuseSocketPath(baseDir)
	lockPath := filepath.Join(baseDir, "fuse.lock")
	if c, err := dialFuseDaemon(baseDir, sockPath); err == nil {
		return c, nil
	}
	unlock, err := lockFuseFile(lockPath)
	if err != nil {
		return nil, fmtMount(err)
	}
	defer unlock()
	if c, err := dialFuseDaemon(baseDir, sockPath); err == nil {
		return c, nil
	}
	if err := spawnFuseDaemon(baseDir); err != nil {
		return nil, err
	}
	c, err := dialFuseDaemon(baseDir, sockPath)
	if err != nil {
		return nil, fmtMount(err)
	}
	return c, nil
}

func runFuseDaemon(baseDir string) {
	mntDir := filepath.Join(baseDir, "mnt")
	sockPath := fuseSocketPath(baseDir)
	lockPath := filepath.Join(baseDir, "fuse.lock")
	if _, err := os.Stat(mntDir); err != nil && !os.IsNotExist(err) {
		detach(mntDir, true)
	}
	if err := os.MkdirAll(mntDir, 0o700); err != nil {
		_, _ = fmt.Fprintf(os.Stdout, "ERR:%s\n", fmtMount(err).Error())
		return
	}
	var root *rootNode
	newRoot := func() fs.InodeEmbedder {
		root = &rootNode{slots: map[string]*viewSlot{}}
		return root
	}
	zero := time.Duration(0)
	discard := log.New(io.Discard, "", 0)
	opts := &fs.Options{
		EntryTimeout:    &zero,
		AttrTimeout:     &zero,
		NullPermissions: true,
		UID:             uint32(os.Getuid()),
		GID:             uint32(os.Getgid()),
		Logger:          discard,
	}
	opts.FsName = "litar"
	opts.Name = "litar"
	opts.MountOptions.Logger = discard
	server, direct, err := mountFUSE(mntDir, newRoot, opts)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stdout, "ERR:%s\n", err.Error())
		return
	}
	_ = os.Remove(sockPath)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		if server.Unmount() != nil {
			detach(mntDir, direct)
		}
		_, _ = fmt.Fprintf(os.Stdout, "ERR:%s\n", fmtMount(err).Error())
		return
	}
	_ = os.Chmod(sockPath, 0o600)
	d := &fuseDaemon{
		baseDir:  baseDir,
		mntDir:   mntDir,
		sockPath: sockPath,
		lockPath: lockPath,
		root:     root,
		server:   server,
		direct:   direct,
		ln:       ln,
		done:     make(chan struct{}),
		nextIno:  2,
		slots:    map[string]*slotState{},
	}
	_, _ = fmt.Fprintln(os.Stdout, "OK")
	_ = os.Stdout.Close()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		d.forceShutdown()
	}()
	go func() {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
			d.mu.Lock()
			idle := !d.everConnected && !d.closing
			d.mu.Unlock()
			if idle {
				d.forceShutdown()
			}
		case <-d.done:
		}
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			<-d.done
			return
		}
		go d.handleConn(conn)
	}
}

func fuseSocketPath(baseDir string) string {
	p := filepath.Join(baseDir, "fuse.sock")
	if len(p) <= 96 {
		return p
	}
	sum := sha256.Sum256([]byte(baseDir))
	return filepath.Join("/tmp", "litar-"+hex.EncodeToString(sum[:8])+".sock")
}
