# notehistory

Keeps a history of every macOS notification, including the ones Notification Centre throws away.

## What it does

Notification Centre deletes a notification's database row the moment it's cleared so you can end up with "What the hell was that notification? Where's it gone?". Some notifications never make it into the panel at all. notehistory watches the live database and copies every new row into its own SQLite file before it can vanish, then gives you a terminal browser to search through what it has kept.

There is a background watcher that runs at login, a `show` command for a quick look, and a TUI with vim-style keys for browsing and searching.

## Prerequisites

macOS. Nothing else if you download a release binary. To build from source you need Go 1.27 or later.

## Getting started

Download the binary for your Mac from the [releases page](https://github.com/ohnotnow/notehistory/releases): `notehistory-darwin-arm64` for Apple Silicon, `notehistory-darwin-amd64` for Intel.

Put it somewhere sensible on your path - ideally your local ~/bin/ or similar.  It makes granting the MacOS permissions easier.

```sh
mkdir -p ~/bin
mv ~/Downloads/notehistory-darwin-arm64 ~/bin/notehistory
chmod +x ~/bin/notehistory
```

Then install the background watcher:

```sh
~/bin/notehistory install
```

Now check it can actually read the notification database:

```sh
~/bin/notehistory status
```

The first time, it almost certainly can't. You need to grant Full Disk Access to the binary:

1. Open System Settings, then Privacy & Security, then Full Disk Access.
2. Click the plus button and pick `~/bin/notehistory` (press Cmd+Shift+G in the file dialog to type a path).
3. Run `notehistory restart`, then `notehistory status` again. The log should now say "watching".

If you build from source instead, `go build -o notehistory .` in the repo produces the same binary.

### Why the fuss about where the binary lives

The launchd agent records the absolute path of the binary, and macOS ties the Full Disk Access grant to it. The Full Disk Access file dialog hides `/usr` and `/opt`, so if you must use `/usr/local/bin` you will need Cmd+Shift+G to reach it.

### Updating

Delete the old binary before copying the new one in. Overwriting it in place confuses the macOS code-signature cache and the next run dies with "Killed: 9".

```sh
rm ~/bin/notehistory
mv ~/Downloads/notehistory-darwin-arm64 ~/bin/notehistory
chmod +x ~/bin/notehistory
notehistory restart
notehistory status
```

Replacing the binary can also drop the Full Disk Access grant. If status shows the watcher cannot read the database, do the System Settings steps again.

## Usage

```
notehistory              browse and search the history interactively
notehistory show [N]     print the last N recorded, oldest first (default 30)
notehistory watch        watch Notification Centre and record everything
notehistory install      run "watch" in the background at login via launchd
notehistory uninstall    stop and remove the background watcher
notehistory restart      restart the background watcher
notehistory status       is the background watcher running, and what its log says
```

Running `notehistory` on its own opens the TUI. Newest notifications are at the top, with the selected one shown in full underneath.

| Key | Action |
|-----|--------|
| `j` / `k` or arrows | move |
| `g` / `G` | top / bottom |
| `ctrl+d` / `ctrl+u` | half a page down / up |
| `/` | search |
| `Enter` | keep the search and go back to the list |
| `Esc` or backspace past the start | clear the search |
| `r` | reload |
| `q` | quit |

Search is incremental. Every word you type has to appear somewhere in the notification, in any order, ignoring case. That covers the app name, title, subtitle, body and timestamp. New notifications turn up while the TUI is open.

App names are looked up from the bundle identifier, so `com.microsoft.teams2` shows as Microsoft Teams. When no installed app claims the identifier you get a tidied-up guess instead.

## Configuration

Three environment variables adjust the defaults.

| Variable | Default |
|----------|---------|
| `NOTIFICATION_DB` | `~/Library/Group Containers/group.com.apple.usernoted/db2/db` |
| `NOTIFICATION_HISTORY` | `~/.local/share/notification-history.db` |
| `NOTIFICATION_KEEP_DAYS` | `365` |

Notifications older than `NOTIFICATION_KEEP_DAYS` are deleted each time the watcher records new ones. Set it to `0` to keep everything.

The background watcher does not see your shell environment. `install` copies any of these variables that are set when you run it into the launchd agent, and `status` shows what it recorded. To change one later, set it and run `install` again.

The watcher logs to `~/Library/Logs/notehistory.log`.

## Development

```sh
go build -o notehistory .
go test ./...
```

Tagging a commit `v*` runs the release workflow, which tests, builds both macOS binaries and publishes them with a `SHA256SUMS` file.

## Contributing

Fork it, clone it, run `go test ./...`, and send a pull request. It is a small codebase: `main.go` dispatches the commands and everything else is in `internal/notehistory`.

## Licence

MIT. See [LICENSE](LICENSE).
