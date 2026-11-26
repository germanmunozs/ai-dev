//go:build darwin

package fswatch

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
)

// kqueue observes each path through one event-only descriptor (R-01). A
// descriptor opened with O_EVTONLY holds no lock and keeps no volume busy;
// O_SYMLINK opens a symbolic link as itself, so it is never followed, and
// such a path is then skipped. O_NONBLOCK keeps a FIFO swapped in for a
// file from blocking the open (review F9).
const vnodeEvents = syscall.NOTE_WRITE | syscall.NOTE_EXTEND | syscall.NOTE_ATTRIB | syscall.NOTE_DELETE | syscall.NOTE_RENAME

// wakeIdent is the user event that wakes the reader on Close.
const wakeIdent = 0

type vnode struct {
	fd       int
	dev, ino uint64
	dir      bool
}

type kqueueWatcher struct {
	kq        int
	events    chan Change
	done      chan struct{}
	exited    chan struct{}
	endEvents sync.Once

	mu     sync.Mutex
	paths  map[string]vnode
	byFD   map[int]string
	closed bool
}

// New starts a kqueue watcher.
func New() (Watcher, error) {
	kq, err := syscall.Kqueue()
	if err != nil {
		return nil, unavailableFor(err)
	}
	syscall.CloseOnExec(kq)
	wake := syscall.Kevent_t{Ident: wakeIdent, Filter: syscall.EVFILT_USER, Flags: syscall.EV_ADD | syscall.EV_CLEAR}
	if _, err := syscall.Kevent(kq, []syscall.Kevent_t{wake}, nil, nil); err != nil {
		syscall.Close(kq)
		return nil, unavailableFor(err)
	}
	w := &kqueueWatcher{kq: kq, events: make(chan Change, 256), done: make(chan struct{}), exited: make(chan struct{}),
		paths: map[string]vnode{}, byFD: map[int]string{}}
	go w.read()
	return w, nil
}

func (w *kqueueWatcher) Events() <-chan Change { return w.events }

func (w *kqueueWatcher) Sync(dirs, files []string) ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, errors.New("watcher closed")
	}
	want := map[string]bool{}
	for _, p := range dirs {
		want[p] = true
	}
	for _, p := range files {
		want[p] = false
	}
	for p, v := range w.paths {
		if _, keep := want[p]; !keep {
			w.forget(p, v)
		}
	}
	var added []string
	for p := range want {
		var st syscall.Stat_t
		if err := syscall.Lstat(p, &st); err != nil || !observable(st.Mode) {
			if v, ok := w.paths[p]; ok {
				w.forget(p, v)
			}
			continue // missing, unreadable, a symbolic link or a special file
		}
		if v, ok := w.paths[p]; ok {
			if v.dev == uint64(st.Dev) && v.ino == st.Ino {
				continue
			}
			w.forget(p, v) // replaced: observe the new file
		}
		ok, err := w.add(p)
		if err != nil {
			return added, err
		}
		if ok {
			added = append(added, p)
		}
	}
	return added, nil
}

// observable reports whether mode is a regular file or a directory.
func observable(mode uint16) bool {
	kind := mode & syscall.S_IFMT
	return kind == syscall.S_IFREG || kind == syscall.S_IFDIR
}

// add opens and registers p. A path that vanished, cannot be read or turned
// into anything but a regular file or directory meanwhile is skipped
// (review F1, F9); only a limit or another system failure is an error.
func (w *kqueueWatcher) add(p string) (bool, error) {
	fd, err := syscall.Open(p, syscall.O_EVTONLY|syscall.O_SYMLINK|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if skippable(err) {
		return false, nil
	}
	if err != nil {
		return false, unavailableFor(err)
	}
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil || !observable(st.Mode) {
		syscall.Close(fd)
		return false, nil
	}
	change := syscall.Kevent_t{Ident: uint64(fd), Filter: syscall.EVFILT_VNODE, Flags: syscall.EV_ADD | syscall.EV_CLEAR, Fflags: vnodeEvents}
	if _, err := syscall.Kevent(w.kq, []syscall.Kevent_t{change}, nil, nil); err != nil {
		syscall.Close(fd)
		return false, unavailableFor(err)
	}
	w.paths[p] = vnode{fd: fd, dev: uint64(st.Dev), ino: st.Ino, dir: st.Mode&syscall.S_IFMT == syscall.S_IFDIR}
	w.byFD[fd] = p
	return true, nil
}

// skippable reports the errors that skip one path rather than stop
// watching: it is gone, unreadable, or a link (review F1).
func skippable(err error) bool {
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ENOTDIR) || errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.ELOOP)
}

// forget closes p's descriptor, which also removes its kevent.
func (w *kqueueWatcher) forget(p string, v vnode) {
	syscall.Close(v.fd)
	delete(w.paths, p)
	delete(w.byFD, v.fd)
}

// read delivers notifications until Close. If it ends any other way, it
// closes Events, so the loop reports that watching was lost (review F8,
// F10).
func (w *kqueueWatcher) read() {
	defer close(w.exited)
	defer func() {
		_ = recover()
		select {
		case <-w.done:
		default:
			w.endEvents.Do(func() { close(w.events) })
		}
	}()
	buf := make([]syscall.Kevent_t, 64)
	for {
		n, err := syscall.Kevent(w.kq, nil, buf, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return
		}
		for _, ev := range buf[:n] {
			if ev.Filter == syscall.EVFILT_USER {
				return // Close
			}
			w.mu.Lock()
			p, known := w.byFD[int(ev.Ident)]
			dir := w.paths[p].dir
			w.mu.Unlock()
			if !known {
				continue
			}
			kind := Changed
			if dir && ev.Fflags&(syscall.NOTE_DELETE|syscall.NOTE_RENAME) != 0 {
				kind = Gone
			}
			select {
			case w.events <- Change{Path: p, Kind: kind}:
			case <-w.done:
				return
			}
		}
	}
}

func (w *kqueueWatcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()
	close(w.done)
	trigger := syscall.Kevent_t{Ident: wakeIdent, Filter: syscall.EVFILT_USER, Fflags: syscall.NOTE_TRIGGER}
	_, _ = syscall.Kevent(w.kq, []syscall.Kevent_t{trigger}, nil, nil)
	<-w.exited
	w.mu.Lock()
	for p, v := range w.paths {
		w.forget(p, v)
	}
	w.mu.Unlock()
	err := syscall.Close(w.kq)
	w.endEvents.Do(func() { close(w.events) })
	return err
}

// unavailableFor maps a system error to Unavailable with its remedy.
func unavailableFor(err error) error {
	if errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) {
		return Unavailable{Reason: "open_file_limit", Detail: fmt.Sprintf("the open-file limit was reached (%v)", err),
			Remedy: "raise the open-file limit (ulimit -n)"}
	}
	return Unavailable{Reason: "watch_failed", Detail: err.Error()}
}
