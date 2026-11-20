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
  slat [-c|--config PATH]             attach to this directory's session (starting it if needed)
  slat [-c|--config PATH] attach [NAME|DIR]
                                      attach to a session by name or directory
  slat [-c|--config PATH] session NAME|PATH
                                      start or attach to a preconfigured session TOML
  slat sessions [--json]              list running sessions
  slat --version                      print the version
  slat --help                         show this help and the command reference

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
	if len(args) > 0 && args[0] == "__daemon" {
		// Hidden: slat re-execs itself with this to become the daemon.
		if err := runDaemon(args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "slat: %v\n", err)
			os.Exit(1)
		}
		return
	}
	configPath, args, err := extractConfigFlag(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "slat: %v\n", err)
		os.Exit(2)
	}
	switch {
	case len(args) == 0:
	case args[0] == "-v" || args[0] == "--version" || args[0] == "version":
		fmt.Println("slat", app.Version)
		return
	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
		fmt.Printf(usage, config.Path(), strings.Join(ui.ThemeNames(), ", "))
		fmt.Print("\n" + cliUsage)
		return
	case args[0] == "sessions":
		os.Exit(runSessions(args[1:]))
	case args[0] == "session":
		sock, dir, presetPath, err := resolveSessionPreset(args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "slat: %v\n", err)
			os.Exit(1)
		}
		if err := runClientWithConfig(sock, dir, presetPath, configPath); err != nil {
			fmt.Fprintf(os.Stderr, "slat: %v\n", err)
			os.Exit(1)
		}
		return
	case args[0] == "attach":
		sock, dir, err := resolveAttach(args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "slat: %v\n", err)
			os.Exit(1)
		}
		if err := runClientWithConfig(sock, dir, "", configPath); err != nil {
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

	if err := runClientWithConfig(socketPath(), "", "", configPath); err != nil {
		fmt.Fprintf(os.Stderr, "slat: %v\n", err)
		os.Exit(1)
	}
}

func extractConfigFlag(args []string) (configPath string, rest []string, err error) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		switch {
		case a == "-c" || a == "--config":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s needs a config file path", a)
			}
			i++
			configPath = args[i]
		case strings.HasPrefix(a, "--config="):
			configPath = strings.TrimPrefix(a, "--config=")
		case strings.HasPrefix(a, "-c="):
			configPath = strings.TrimPrefix(a, "-c=")
		default:
			rest = append(rest, a)
		}
	}
	configPath = strings.TrimSpace(configPath)
	if configPath != "" {
		if abs, absErr := filepath.Abs(configPath); absErr == nil {
			configPath = abs
		}
	}
	return configPath, rest, nil
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

func resolveSessionPreset(args []string) (sock, dir, presetPath string, err error) {
	if len(args) != 1 {
		return "", "", "", fmt.Errorf("usage: slat session <NAME|PATH>")
	}
	presetPath, err = session.ResolvePresetPath(args[0])
	if err != nil {
		return "", "", "", err
	}
	preset, err := session.LoadPreset(presetPath)
	if err != nil {
		return "", "", "", err
	}
	dir = session.ExpandPath(preset.StartDirectory)
	if dir == "" {
		dir = session.CanonicalDir("")
	} else {
		dir = session.CanonicalDir(dir)
	}
	return socketPathForDir(dir), dir, presetPath, nil
}

func runDaemon(args []string) error {
	var presetPath, configPath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			if i+1 < len(args) {
				i++
				presetPath = args[i]
			}
		case "--config":
			if i+1 < len(args) {
				i++
				configPath = args[i]
			}
		}
	}
	var (
		cfg *config.Config
		err error
	)
	if configPath != "" {
		cfg, err = config.LoadFrom(configPath)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		return err
	}
	srv, err := daemon.New(cfg, socketPath())
	if err != nil {
		return err
	}
	if presetPath != "" {
		p, err := session.LoadPreset(presetPath)
		if err != nil {
			return err
		}
		srv.SetPreset(p)
	}
	return srv.Run()
}

func runClientWithConfig(sock, dir, presetPath, configPath string) error {
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
			var err error
			if configPath != "" {
				_, err = config.LoadFrom(configPath)
			} else {
				_, err = config.Load()
			}
			if err != nil {
				return err
			}
			if err := spawnDaemon(sock, dir, presetPath, configPath); err != nil {
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

func spawnDaemon(sock, dir, presetPath, configPath string) error {
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
	daemonArgs := []string{"__daemon"}
	if presetPath != "" {
		if abs, absErr := filepath.Abs(presetPath); absErr == nil {
			presetPath = abs
		}
		daemonArgs = append(daemonArgs, "--session", presetPath)
	}
	if configPath != "" {
		daemonArgs = append(daemonArgs, "--config", configPath)
	}
	cmd := exec.Command(exe, daemonArgs...)
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
