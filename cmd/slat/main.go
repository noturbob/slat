package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/noturbob/slat/internal/app"
	"github.com/noturbob/slat/internal/client"
	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/control"
	"github.com/noturbob/slat/internal/daemon"
	"github.com/noturbob/slat/internal/session"
	"github.com/noturbob/slat/internal/ui"
)

const usage = `slat — a terminal multiplexer

usage:
  slat                   attach to this directory's session (starting it if needed)
  slat attach [NAME|DIR] attach to a session by name or directory
  slat sessions [--json] list running sessions
  slat --version         print the version
  slat --help            show this help and the command reference

Inside slat, press the prefix (Ctrl-S by default) then ? for keybindings.
Config: %s
Themes: %s (set theme.name; any colour can be overridden)
`

func socketPath() string { return socketPathForDir("") }

func cliSocketPath() string {
	if s := os.Getenv("SLAT"); s != "" {
		return s
	}
	return socketPath()
}

// socketPathForDir is the per-user, per-directory daemon socket.
func socketPathForDir(sessionDir string) string {
	baseName := strings.TrimSuffix(socketName(), ".sock")
	name := baseName + "-" + session.DirHash(sessionDir) + ".sock"
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		if p := filepath.Join(dir, name); len(p) < 104 {
			return p
		}
	}
	sub := fmt.Sprintf("slat-%d", os.Getuid())
	for _, base := range []string{os.TempDir(), "/tmp"} {
		dir := filepath.Join(base, sub)
		if p := filepath.Join(dir, name); len(p) < 104 {
			if err := os.MkdirAll(dir, 0o700); err == nil {
				return p
			}
		}
		if p := filepath.Join(base, name); len(p) < 104 {
			return p
		}
	}
	return filepath.Join("/tmp", name)
}

func socketDirs() []string {
	seen := map[string]bool{}
	var dirs []string
	add := func(d string) {
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	add(os.Getenv("XDG_RUNTIME_DIR"))
	sub := fmt.Sprintf("slat-%d", os.Getuid())
	add(filepath.Join(os.TempDir(), sub))
	add(filepath.Join("/tmp", sub))
	add(os.TempDir())
	add("/tmp")
	return dirs
}

func listSessions() []app.SessionSummary {
	basePrefix := strings.TrimSuffix(socketName(), ".sock")
	return control.ListSessions(socketDirs(), basePrefix)
}

func main() {
	// Release builds stamp app.Version; `go install …@v1.2.3` records the
	// module version in the binary instead.
	if info, ok := debug.ReadBuildInfo(); ok && app.Version == "dev" && info.Main.Version != "(devel)" && info.Main.Version != "" {
		app.Version = info.Main.Version
	}
	args := os.Args[1:]
	switch {
	case len(args) == 0:
	case args[0] == "__daemon":
		// Hidden: slat re-execs itself with this to become the daemon.
		if err := runDaemon(); err != nil {
			fmt.Fprintf(os.Stderr, "slat: %v\n", err)
			os.Exit(1)
		}
		return
	case args[0] == "-v" || args[0] == "--version" || args[0] == "version":
		fmt.Println("slat", app.Version)
		return
	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
		fmt.Printf(usage, config.Path(), strings.Join(ui.ThemeNames(), ", "))
		fmt.Print("\n" + cliUsage)
		return
	case args[0] == "sessions":
		os.Exit(runSessions(args[1:]))
	case args[0] == "attach":
		sock, dir, err := resolveAttach(args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "slat: %v\n", err)
			os.Exit(1)
		}
		if err := runClientAt(sock, dir); err != nil {
			fmt.Fprintf(os.Stderr, "slat: %v\n", err)
			os.Exit(1)
		}
		return
	default:
		// Agent-facing commands talk to a running session over the socket.
		if handled, code := runCLI(cliSocketPath(), args); handled {
			os.Exit(code)
		}
		fmt.Fprintf(os.Stderr, "slat: unknown argument %q (try slat --help)\n", args[0])
		os.Exit(2)
	}

	if err := runClient(); err != nil {
		fmt.Fprintf(os.Stderr, "slat: %v\n", err)
		os.Exit(1)
	}
}

func resolveAttach(args []string) (sock, dir string, err error) {
	if len(args) == 0 {
		return socketPath(), "", nil
	}
	target := args[0]
	if fi, stErr := os.Stat(target); stErr == nil {
		if fi.IsDir() {
			abs := session.CanonicalDir(target)
			return socketPathForDir(abs), abs, nil
		}
		if fi.Mode()&os.ModeSocket != 0 {
			return target, "", nil
		}
	}
	for _, s := range listSessions() {
		if s.Name == target || filepath.Base(s.Socket) == target {
			return s.Socket, "", nil
		}
	}
	return "", "", fmt.Errorf("no session %q (run 'slat sessions' to list)", target)
}

func runDaemon() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	srv, err := daemon.New(cfg, socketPath())
	if err != nil {
		return err
	}
	return srv.Run()
}

func runClient() error {
	return runClientAt(socketPath(), "")
}

func runClientAt(sock, dir string) error {
	if os.Getenv("SLAT") != "" {
		return fmt.Errorf("already inside a slat session (unset SLAT to force)")
	}
	for {
		if !daemonAlive(sock) {
			if dir == "" && sock != socketPath() {
				return fmt.Errorf("session socket %s is not running", sock)
			}
			// The daemon has no terminal to report errors on, so catch a
			// broken config here, where the user can see it.
			if _, err := config.Load(); err != nil {
				return err
			}
			if err := spawnDaemon(sock, dir); err != nil {
				return fmt.Errorf("failed to start daemon: %w", err)
			}
			if err := waitForSocket(sock, 3*time.Second); err != nil {
				return fmt.Errorf("%w (see %s.log)", err, sock)
			}
		}
		detached, err := client.Run(sock)
		if err != nil {
			return err
		}
		if detached {
			if resp, popErr := control.Do(sock, control.Request{Cmd: "pop-switch"}, 500*time.Millisecond); popErr == nil && resp.SwitchTo != "" {
				if exe, exErr := os.Executable(); exErr == nil {
					if err := execAttach(exe, resp.SwitchTo); err != nil {
						return err
					}
				}
				sock = resp.SwitchTo
				dir = ""
				continue
			}
			fmt.Println("[detached — run slat to reattach]")
		} else {
			fmt.Println("[slat session ended]")
		}
		return nil
	}
}

func daemonAlive(sock string) bool {
	c, err := net.DialTimeout("unix", sock, 200*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func spawnDaemon(sock, dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer devnull.Close()
	// The daemon has no terminal; its errors and any crash trace go here.
	logf, err := os.OpenFile(sock+".log", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd := exec.Command(exe, "__daemon")
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, logf, logf
	cmd.SysProcAttr = detachAttrs()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func waitForSocket(sock string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if daemonAlive(sock) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not start within %s", timeout)
}
