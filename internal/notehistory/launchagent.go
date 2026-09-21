package notehistory

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"howett.net/plist"
)

const agentLabel = "dev.notehistory.watch"

var agentPlist = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.Label}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.Binary}}</string>
		<string>watch</string>
	</array>
{{if .Env}}	<key>EnvironmentVariables</key>
	<dict>
{{range $k, $v := .Env}}		<key>{{$k}}</key>
		<string>{{$v}}</string>
{{end}}	</dict>
{{end}}	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>{{.Log}}</string>
	<key>StandardErrorPath</key>
	<string>{{.Log}}</string>
</dict>
</plist>
`))

func agentPaths() (plistPath, logPath string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(home, "Library/LaunchAgents", agentLabel+".plist"),
		filepath.Join(home, "Library/Logs/notehistory.log"), nil
}

// InstallAgent writes a LaunchAgent that runs this binary's watch command at
// login and keeps it alive, then loads it. Because launchd starts the binary
// directly, Full Disk Access is granted to the binary itself rather than to
// whichever terminal happened to run it.
func InstallAgent() error {
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	if binary, err = filepath.EvalSymlinks(binary); err != nil {
		return err
	}
	plistPath, logPath, err := agentPaths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(plistPath)
	if err != nil {
		return err
	}
	env := map[string]string{}
	for _, name := range envVars {
		if v := os.Getenv(name); v != "" {
			env[name] = v
		}
	}
	err = agentPlist.Execute(f, map[string]any{"Label": agentLabel, "Binary": binary, "Log": logPath, "Env": env})
	f.Close()
	if err != nil {
		return err
	}
	// Bootout first so re-running install picks up a moved binary.
	exec.Command("launchctl", "bootout", domain(), plistPath).Run()
	if out, err := exec.Command("launchctl", "bootstrap", domain(), plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %v: %s", err, out)
	}
	fmt.Printf("installed %s\nrunning %s watch, logging to %s\n", plistPath, binary, logPath)
	for k, v := range env {
		fmt.Printf("  with %s=%s\n", k, v)
	}
	fmt.Printf("\nIf the log says it cannot read the notification database, grant Full Disk Access to:\n  %s\nin System Settings > Privacy & Security > Full Disk Access, then run: %s restart\n", binary, filepath.Base(binary))
	fmt.Print("macOS forgets the grant whenever the binary is replaced, so repeat this after updating. Check with: notehistory status\n")
	fmt.Print(locationAdvice(binary))
	return nil
}

// locationAdvice warns about binary locations that make life harder: system
// directories are awkward to reach in the Full Disk Access file dialog, and
// Downloads tends to get emptied.
func locationAdvice(binary string) string {
	switch {
	case strings.HasPrefix(binary, "/usr/") || strings.HasPrefix(binary, "/opt/"):
		return "\nNote: the Full Disk Access dialog hides " + filepath.Dir(binary) + ". Press Cmd+Shift+G in the dialog and type the path,\n" +
			"or keep the binary somewhere under your home directory (such as ~/bin) and run install again.\n"
	case strings.Contains(binary, "/Downloads/"):
		return "\nNote: the binary is in Downloads, which tends to get cleared out. Move it somewhere permanent\n" +
			"(such as ~/bin) and run install again.\n"
	}
	return ""
}

// UninstallAgent unloads and removes the LaunchAgent.
func UninstallAgent() error {
	plistPath, _, err := agentPaths()
	if err != nil {
		return err
	}
	if agentState().loaded {
		if out, err := exec.Command("launchctl", "bootout", domain(), plistPath).CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl bootout: %v: %s", err, out)
		}
		fmt.Println("stopped the background watcher")
	}
	if err := os.Remove(plistPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("nothing to remove: the background watcher was not installed")
			return nil
		}
		return err
	}
	fmt.Println("removed", plistPath)
	return nil
}

// state is what launchd reports about the agent.
type state struct {
	loaded   bool
	running  bool
	pid      string
	lastExit string
	program  string
}

// agentState asks launchd about the agent. A missing service is reported as
// not loaded rather than as an error.
func agentState() state {
	out, err := exec.Command("launchctl", "print", domain()+"/"+agentLabel).Output()
	if err != nil {
		return state{}
	}
	st := state{loaded: true}
	seenState := false
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " = ")
		if !ok {
			continue
		}
		switch key {
		case "state":
			// The first "state" line is the service's own; later ones
			// belong to nested endpoints and are ignored.
			if !seenState {
				st.running = value == "running"
				seenState = true
			}
		case "pid":
			st.pid = value
		case "last exit code":
			st.lastExit = value
		case "program":
			st.program = value
		}
	}
	return st
}

// Status prints whether the background watcher is installed and running,
// and the tail of its log with a hint when the log shows the usual
// Full Disk Access failure.
func Status() error {
	plistPath, logPath, err := agentPaths()
	if err != nil {
		return err
	}
	st := agentState()
	fmt.Println("background watcher:")
	switch {
	case !st.loaded:
		if _, err := os.Stat(plistPath); err == nil {
			fmt.Printf("  installed but not loaded (%s)\n  run: notehistory install\n", plistPath)
		} else {
			fmt.Println("  not installed\n  run: notehistory install")
		}
	case st.running:
		fmt.Printf("  running (pid %s)\n", st.pid)
	default:
		fmt.Printf("  loaded but not running (last exit code %s)\n", st.lastExit)
	}
	if st.program != "" {
		fmt.Printf("binary:\n  %s\n", st.program)
	}
	if env := plistEnv(plistPath); len(env) > 0 {
		fmt.Println("environment:")
		for k, v := range env {
			fmt.Printf("  %s=%s\n", k, v)
		}
	}

	fmt.Printf("log:\n  %s\n", logPath)
	lines, err := tailLines(logPath, 5)
	if err != nil {
		fmt.Println("  (no log yet)")
		return nil
	}
	for _, l := range lines {
		fmt.Println("  " + l)
	}
	if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "cannot read ") {
		fmt.Printf("\nThe watcher cannot read Notification Centre's database. Grant Full Disk Access to\n  %s\nin System Settings > Privacy & Security > Full Disk Access, then run: notehistory restart\n"+
			"macOS forgets the grant whenever the binary is replaced, so repeat this after updating.\n", st.program)
	}
	return nil
}

func tailLines(path string, n int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// RestartAgent kicks the running agent so it restarts, for example after
// Full Disk Access has been granted.
func RestartAgent() error {
	out, err := exec.Command("launchctl", "kickstart", "-k", domain()+"/"+agentLabel).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl kickstart: %v: %s", err, out)
	}
	return nil
}

func domain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

// plistEnv reads the EnvironmentVariables baked into the agent plist.
func plistEnv(plistPath string) map[string]string {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return nil
	}
	var agent struct {
		Env map[string]string `plist:"EnvironmentVariables"`
	}
	if _, err := plist.Unmarshal(data, &agent); err != nil {
		return nil
	}
	return agent.Env
}
