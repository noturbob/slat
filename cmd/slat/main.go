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
	"github.com/noturbob/slat/internal/daemon"
	"github.com/noturbob/slat/internal/ui"
)

const usage = `slat — a terminal multiplexer

usage:
  slat             attach to your session (starting it if needed)
  slat --version   print the version
  slat --help      show this help and the command reference

Inside slat, press the prefix (Ctrl-S by default) then ? for keybindings.
Config: %s
Themes: %s (set theme.name; any colour can be overridden)
`

// socketPath is the per-user daemon socket. $XDG_RUNTIME_DIR is private to
// the user; the shared temp dir is the fallback.
func socketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	name := socketName()
	// Socket paths can't exceed 104 bytes (macOS; 108 on Linux), and a
	// long $TMPDIR would otherwise fail with "bind: invalid argument".
	if p := filepath.Join(dir, name); len(p) < 104 {
		return p
	}
	return filepath.Join("/tmp", name)
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
	default:
		// Agent-facing commands talk to a running session over the socket.
		if handled, code := runCLI(socketPath(), args); handled {
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
	if os.Getenv("SLAT") != "" {
		return fmt.Errorf("already inside a slat session (unset SLAT to force)")
	}
	sock := socketPath()
	if !daemonAlive(sock) {
		// The daemon has no terminal to report errors on, so catch a
		// broken config here, where the user can see it.
		if _, err := config.Load(); err != nil {
			return err
		}
		if err := spawnDaemon(sock); err != nil {
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
		fmt.Println("[detached — run slat to reattach]")
	} else {
		fmt.Println("[slat session ended]")
	}
	return nil
}

func daemonAlive(sock string) bool {
	c, err := net.DialTimeout("unix", sock, 200*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func spawnDaemon(sock string) error {
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
