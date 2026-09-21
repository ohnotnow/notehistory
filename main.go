// notehistory keeps a history of macOS notifications.
//
// Notification Centre deletes a notification's row the moment it is cleared
// or withdrawn, so the panel is not a history. This watches the live
// database's WAL file with kqueue and copies every new row into its own
// SQLite file before it can vanish.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/ohnotnow/notehistory/internal/notehistory"
)

const usage = `usage:
  notehistory              browse and search the history interactively
  notehistory show [N]     print the last N recorded, oldest first (default 30)
  notehistory watch        watch Notification Centre and record everything
  notehistory install      run "watch" in the background at login via launchd
  notehistory uninstall    stop and remove the background watcher
  notehistory restart      restart the background watcher
  notehistory status       is the background watcher running, and what its log says
  notehistory version

Environment: NOTIFICATION_DB and NOTIFICATION_HISTORY override the paths.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "tui"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	cfg := notehistory.LoadConfig()
	switch cmd {
	case "watch":
		return watch(cfg)
	case "show":
		return show(cfg, args)
	case "tui":
		store, err := notehistory.OpenStore(cfg.HistoryDB)
		if err != nil {
			return err
		}
		defer store.Close()
		return notehistory.RunTUI(store)
	case "install":
		return notehistory.InstallAgent()
	case "uninstall":
		return notehistory.UninstallAgent()
	case "restart":
		return notehistory.RestartAgent()
	case "status":
		return notehistory.Status()
	case "version", "--version":
		fmt.Println("notehistory", notehistory.Version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
		return nil
	}
}

func watch(cfg notehistory.Config) error {
	store, err := notehistory.OpenStore(cfg.HistoryDB)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	names := notehistory.NewAppNames()
	return notehistory.Watch(ctx, cfg, store, func(n notehistory.Notification) {
		fmt.Println(n.Line(names.Name(n.App)))
	})
}

func show(cfg notehistory.Config, args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	limit := 30
	if fs.NArg() > 0 {
		n, err := strconv.Atoi(fs.Arg(0))
		if err != nil {
			return fmt.Errorf("show: %q is not a number", fs.Arg(0))
		}
		limit = n
	}
	store, err := notehistory.OpenStore(cfg.HistoryDB)
	if err != nil {
		return err
	}
	defer store.Close()
	rows, err := store.Recent(context.Background(), limit)
	if err != nil {
		return err
	}
	names := notehistory.NewAppNames()
	// Oldest first, so the newest is at the bottom and scrolling back in
	// the terminal means going back in time.
	for i := len(rows) - 1; i >= 0; i-- {
		fmt.Println(rows[i].Line(names.Name(rows[i].App)))
	}
	return nil
}
