package notehistory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// rescanInterval is a belt-and-braces rescan alongside kqueue.
const rescanInterval = 60 * time.Second

const (
	kqGone    = unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_REVOKE
	kqChanged = unix.NOTE_WRITE | unix.NOTE_EXTEND | kqGone
)

// Watch records everything currently in Notification Centre, then keeps
// watching its WAL file with kqueue and records new rows as they land.
// Each newly recorded notification is passed to onNew.
func Watch(ctx context.Context, cfg Config, store *Store, onNew func(Notification)) error {
	sync := func() error {
		rows, err := CurrentRows(ctx, cfg.SourceDB)
		if err != nil {
			return err
		}
		added, err := store.Record(ctx, rows)
		if err != nil {
			return err
		}
		for _, n := range added {
			onNew(n)
		}
		if cfg.KeepDays > 0 {
			cutoff := time.Now().AddDate(0, 0, -cfg.KeepDays)
			if _, err := store.Prune(ctx, cutoff); err != nil {
				return fmt.Errorf("prune: %w", err)
			}
		}
		return nil
	}
	if err := sync(); err != nil {
		return fmt.Errorf("cannot read %s: %w (does this program have Full Disk Access?)", cfg.SourceDB, err)
	}

	kq, err := unix.Kqueue()
	if err != nil {
		return err
	}
	defer unix.Close(kq)
	wal := cfg.SourceDB + "-wal"
	fd, err := openWAL(ctx, wal)
	if err != nil {
		return err
	}
	if err := subscribe(kq, fd); err != nil {
		return err
	}
	// Go restarts syscalls interrupted by signals, so a cancelled context
	// would not wake Kevent on its own. A user event does that job.
	if err := addWakeup(kq); err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		triggerWakeup(kq)
	}()
	fmt.Fprintf(os.Stderr, "watching %s, recording to %s\n", wal, cfg.HistoryDB)

	events := make([]unix.Kevent_t, 1)
	timeout := unix.NsecToTimespec(rescanInterval.Nanoseconds())
	for {
		n, err := unix.Kevent(kq, nil, events, &timeout)
		if err != nil && !errors.Is(err, unix.EINTR) {
			return err
		}
		if ctx.Err() != nil {
			break
		}
		if err := sync(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		if n > 0 && events[0].Filter == unix.EVFILT_VNODE && events[0].Fflags&kqGone != 0 {
			unix.Close(fd) // closing the fd drops its kevent too
			if fd, err = openWAL(ctx, wal); err != nil {
				return err
			}
			if err := subscribe(kq, fd); err != nil {
				return err
			}
		}
	}
	unix.Close(fd)
	return nil
}

func openWAL(ctx context.Context, path string) (int, error) {
	for {
		fd, err := unix.Open(path, unix.O_RDONLY, 0)
		if err == nil {
			return fd, nil
		}
		if !errors.Is(err, unix.ENOENT) {
			return 0, err
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func subscribe(kq, fd int) error {
	ev := unix.Kevent_t{
		Ident:  uint64(fd),
		Filter: unix.EVFILT_VNODE,
		Flags:  unix.EV_ADD | unix.EV_CLEAR,
		Fflags: kqChanged,
	}
	_, err := unix.Kevent(kq, []unix.Kevent_t{ev}, nil, nil)
	return err
}

const wakeupIdent = 1

func addWakeup(kq int) error {
	ev := unix.Kevent_t{Ident: wakeupIdent, Filter: unix.EVFILT_USER, Flags: unix.EV_ADD | unix.EV_CLEAR}
	_, err := unix.Kevent(kq, []unix.Kevent_t{ev}, nil, nil)
	return err
}

func triggerWakeup(kq int) {
	ev := unix.Kevent_t{Ident: wakeupIdent, Filter: unix.EVFILT_USER, Fflags: unix.NOTE_TRIGGER}
	unix.Kevent(kq, []unix.Kevent_t{ev}, nil, nil)
}
