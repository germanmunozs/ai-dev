//go:build linux

package fswatch

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
)

// inotify reports the events of a directory's entries, so only directories
// are observed (R-01); IN_DONT_FOLLOW and IN_ONLYDIR mean a symbolic link
// is never followed.
const dirEvents = syscall.IN_CREATE | syscall.IN_DELETE | syscall.IN_MOVED_FROM | syscall.IN_MOVED_TO | syscall.IN_MODIFY |
	syscall.IN_CLOSE_WRITE | syscall.IN_ATTRIB | syscall.IN_DELETE_SELF | syscall.IN_MOVE_SELF |
	syscall.IN_DONT_FOLLOW | syscall.IN_ONLYDIR

// inotifyInit and addWatch are variables so tests can inject the limits
// (init is reserved in Go, analysis I5).
var (
	inotifyInit = func() (int, error) { return syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK) }
	addWatch    = syscall.InotifyAddWatch
)

type watchedDir struct {
	wd       int
	dev, ino uint64
}

type inotifyWatcher struct {
	fd     int
	file   *os.File // the descriptor in the runtime poller, so Close ends a blocked Read
	events chan Change
	done   chan struct{}
	exited chan struct{}

	mu     sync.Mutex
	dirs   map[string]watchedDir
	byWD   map[int32]string
	closed bool
}

// New starts an inotify watcher.
func New() (Watcher, error) {
	fd, err := inotifyInit()
	if err != nil {
		if errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) {
			return nil, Unavailable{Reason: "instance_limit", Detail: fmt.Sprintf("the inotify instance limit was reached (fs.inotify.max_user_instances): %v", err),
				Remedy: "sudo sysctl fs.inotify.max_user_instances=<n>, or close other watching programs"}
		}
		return nil, Unavailable{Reason: "watch_failed", Detail: err.Error()}
	}
	w := &inotifyWatcher{fd: fd, file: os.NewFile(uintptr(fd), "inotify"), events: make(chan Change, 256),
		done: make(chan struct{}), exited: make(chan struct{}), dirs: map[string]watchedDir{}, byWD: map[int32]string{}}
	go w.read()
	return w, nil
}

func (w *inotifyWatcher) Events() <-chan Change { return w.events }

// Sync observes dirs; files are ignored, since a directory's watch reports
// its entries.
func (w *inotifyWatcher) Sync(dirs, _ []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("watcher closed")
	}
	want := map[string]bool{}
	for _, p := range dirs {
		want[p] = true
	}
	for p, d := range w.dirs {
		if !want[p] {
			w.forget(p, d)
		}
	}
	for p := range want {
		var st syscall.Stat_t
		if err := syscall.Lstat(p, &st); err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFDIR {
			if d, ok := w.dirs[p]; ok {
				w.forget(p, d)
			}
			continue // missing, a symbolic link, or not a directory
		}
		if d, ok := w.dirs[p]; ok {
			if d.dev == uint64(st.Dev) && d.ino == st.Ino {
				continue
			}
			w.forget(p, d)
		}
		wd, err := addWatch(w.fd, p, dirEvents)
		if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ENOTDIR) {
			continue
		}
		if errors.Is(err, syscall.ENOSPC) {
			return Unavailable{Reason: "watch_limit", Detail: "the inotify watch limit was reached (fs.inotify.max_user_watches)",
				Remedy: "sudo sysctl fs.inotify.max_user_watches=524288"}
		}
		if err != nil {
			return Unavailable{Reason: "watch_failed", Detail: err.Error()}
		}
		w.dirs[p] = watchedDir{wd: wd, dev: uint64(st.Dev), ino: st.Ino}
		w.byWD[int32(wd)] = p
	}
	return nil
}

func (w *inotifyWatcher) forget(p string, d watchedDir) {
	_, _ = syscall.InotifyRmWatch(w.fd, uint32(d.wd))
	delete(w.dirs, p)
	delete(w.byWD, int32(d.wd))
}

func (w *inotifyWatcher) pathOf(wd int32) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.byWD[wd]
	return p, ok
}

func (w *inotifyWatcher) read() {
	defer close(w.exited)
	buf := make([]byte, 64*(syscall.SizeofInotifyEvent+syscall.NAME_MAX+1))
	for {
		n, err := w.file.Read(buf)
		if err != nil {
			return // closed
		}
		for _, c := range parseEvents(buf[:n], w.pathOf) {
			select {
			case w.events <- c:
			case <-w.done:
				return
			}
		}
	}
}

// parseEvents turns inotify records into changes of the watched
// directories. IN_IGNORED, sent when a watch is removed, is skipped.
func parseEvents(buf []byte, pathOf func(int32) (string, bool)) []Change {
	var changes []Change
	for len(buf) >= syscall.SizeofInotifyEvent {
		wd := int32(binary.NativeEndian.Uint32(buf[0:4]))
		mask := binary.NativeEndian.Uint32(buf[4:8])
		nameLen := int(binary.NativeEndian.Uint32(buf[12:16]))
		next := syscall.SizeofInotifyEvent + nameLen
		if next > len(buf) {
			break
		}
		buf = buf[next:]
		switch {
		case mask&syscall.IN_Q_OVERFLOW != 0:
			changes = append(changes, Change{Kind: Overflow})
		case mask&syscall.IN_IGNORED != 0:
		default:
			p, ok := pathOf(wd)
			if !ok {
				continue
			}
			kind := Changed
			if mask&(syscall.IN_DELETE_SELF|syscall.IN_MOVE_SELF) != 0 {
				kind = Gone
			}
			changes = append(changes, Change{Path: p, Kind: kind})
		}
	}
	return changes
}

func (w *inotifyWatcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()
	close(w.done)
	err := w.file.Close() // ends the blocked Read and removes every watch
	<-w.exited
	close(w.events)
	return err
}
