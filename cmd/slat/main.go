package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jxskiss/mcli"
	"github.com/noturbob/slat/internal/app"
	"github.com/noturbob/slat/internal/client"
	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/control"
	"github.com/noturbob/slat/internal/daemon"
	"github.com/noturbob/slat/internal/session"
	"github.com/noturbob/slat/internal/ui"
)

type globalFlags struct {
	Config  string `cli:"-c, --config, path to config file"`
	Version bool   `cli:"-v, --version, print the version"`
}

type cliDeps struct {
	doControl     func(sock string, req control.Request, timeout time.Duration) (control.Response, error)
	listSessions  func() []app.SessionSummary
	defaultSocket func() string
	runClient     func(sock, dir, presetPath, configPath string) error
	runDaemon     func(presetPath, configPath string) error
	resolveAttach func(target string) (sock, dir string, err error)
	resolvePreset func(target string) (sock, dir, presetPath string, err error)
	out           io.Writer
	errOut        io.Writer
	exit          func(int)
	continueOnErr bool
}

func defaultDeps() cliDeps {
	return cliDeps{
		doControl:     control.Do,
		listSessions:  listSessions,
		defaultSocket: cliSocketPath,
		runClient:     runClientWithConfig,
		runDaemon:     runDaemon,
		resolveAttach: resolveAttach,
		resolvePreset: resolveSessionPreset,
		out:           os.Stdout,
		errOut:        os.Stderr,
		exit:          os.Exit,
	}
}

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
	m, _ := newCLIApp(defaultDeps())
	m.Run(os.Args[1:]...)
}

func resolveConfigPath(raw string) string {
	p := strings.TrimSpace(raw)
	if p != "" {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
	}
	return p
}

func addCmd[T any](
	m *mcli.App,
	name, desc string,
	maxPositional int,
	check func(*mcli.Context, int) bool,
	parseOpts []mcli.ParseOpt,
	fn func(*mcli.Context, *T),
	cmdOpts ...mcli.CmdOpt,
) {
	m.Add(name, mcli.NewCommand(func(ctx *mcli.Context, args *T) {
		if check(ctx, maxPositional) {
			fn(ctx, args)
		}
	}, parseOpts...), desc, cmdOpts...)
}

func newCLIApp(deps cliDeps) (*mcli.App, *globalFlags) {
	m := mcli.NewApp()
	m.Description = "slat — a terminal multiplexer"
	userThemes, _ := ui.LoadThemesFromDir(config.ThemesDir())
	m.Options = mcli.Options{
		KeepCommandOrder: true,
		HelpFooter: fmt.Sprintf(
			"Inside slat, press the prefix (Ctrl-S by default) then ? for keybindings.\nConfig: %s\nThemes dir: %s\nThemes: %s (set theme.name; any colour can be overridden)",
			config.Path(), config.ThemesDir(), strings.Join(ui.ThemeNamesFrom(userThemes), ", "),
		),
	}

	var globals globalFlags
	m.SetGlobalFlags(&globals)

	var parseOpts []mcli.ParseOpt
	if deps.continueOnErr {
		parseOpts = append(parseOpts, mcli.WithErrorHandling(flag.ContinueOnError))
	}

	fail := func(code int, err error) {
		writef(deps.errOut, "slat: %v\n", err)
		deps.exit(code)
	}

	checkCmd := func(ctx *mcli.Context, maxPositional int) bool {
		if err := ctx.ArgsError(); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				deps.exit(0)
				return false
			}
			fail(2, err)
			return false
		}
		if maxPositional >= 0 && ctx.FlagSet().NArg() > maxPositional {
			fail(2, fmt.Errorf("unexpected argument %q (try slat --help)", ctx.FlagSet().Arg(maxPositional)))
			return false
		}
		if globals.Version {
			writeln(deps.out, "slat", app.Version)
			deps.exit(0)
			return false
		}
		return true
	}

	type rootArgs struct{}
	m.AddRoot(mcli.NewCommand(func(ctx *mcli.Context, _ *rootArgs) {
		if !checkCmd(ctx, 0) {
			return
		}
		if err := deps.runClient(socketPath(), "", "", resolveConfigPath(globals.Config)); err != nil {
			fail(1, err)
		}
	}, parseOpts...))

	catSession := mcli.WithCategory("Session")

	type attachArgs struct {
		Target string `cli:"target, session name, directory, or socket path"`
	}
	addCmd(m, "attach", "attach to a session by name or directory", 1, checkCmd, parseOpts,
		func(_ *mcli.Context, args *attachArgs) {
			sock, dir, err := deps.resolveAttach(args.Target)
			if err != nil {
				fail(1, err)
				return
			}
			if clientErr := deps.runClient(sock, dir, "", resolveConfigPath(globals.Config)); clientErr != nil {
				fail(1, clientErr)
			}
		}, catSession)

	type sessionsArgs struct {
		JSONFlag
	}
	// Registered before "session" so mcli's prefix matcher doesn't indent it under "session".
	addCmd(m, "sessions", "list running sessions", 0, checkCmd, parseOpts,
		func(_ *mcli.Context, args *sessionsArgs) {
			sessions := deps.listSessions()
			if args.JSON {
				b, err := json.Marshal(control.Response{Schema: control.Schema, Sessions: sessions})
				if err != nil {
					fail(1, err)
					return
				}
				writeln(deps.out, string(b))
				return
			}
			if len(sessions) == 0 {
				writeln(deps.out, "no active sessions")
				return
			}
			for _, s := range sessions {
				state := "detached"
				if s.Attached {
					state = "attached"
				}
				writef(deps.out, "%-14s  %d tabs  %d panes  %-8s  %s\n", s.Name, s.Tabs, s.Panes, state, s.Dir)
			}
		}, catSession)

	type sessionArgs struct {
		Target string `cli:"#R, target, preconfigured session name or TOML path"`
	}
	addCmd(m, "session", "start or attach to a preconfigured session TOML", 1, checkCmd, parseOpts,
		func(_ *mcli.Context, args *sessionArgs) {
			sock, dir, presetPath, err := deps.resolvePreset(args.Target)
			if err != nil {
				fail(1, err)
				return
			}
			if clientErr := deps.runClient(sock, dir, presetPath, resolveConfigPath(globals.Config)); clientErr != nil {
				fail(1, clientErr)
			}
		}, catSession)

	type versionArgs struct{}
	addCmd(m, "version", "print the version", 0, checkCmd, parseOpts,
		func(_ *mcli.Context, _ *versionArgs) {
			writeln(deps.out, "slat", app.Version)
		})

	type daemonArgs struct {
		Session string `cli:"--session, path to session preset TOML"`
	}
	// Hidden: slat re-execs itself with this to become the daemon.
	m.AddHidden("__daemon", mcli.NewCommand(func(ctx *mcli.Context, args *daemonArgs) {
		if !checkCmd(ctx, 0) {
			return
		}
		if err := deps.runDaemon(args.Session, resolveConfigPath(globals.Config)); err != nil {
			fail(1, err)
		}
	}, parseOpts...), "run the background daemon")

	registerAgentCommands(m, deps, checkCmd, fail, parseOpts)
	registerThemeCommands(m, deps, checkCmd, fail, parseOpts, &globals)
	m.EnableFlagCompletionForAllCommands = true
	m.AddCompletion()
	m.AddHelp()
	return m, &globals
}

func resolveAttach(target string) (string, string, error) {
	if target == "" {
		return socketPath(), "", nil
	}
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

func resolveSessionPreset(target string) (string, string, string, error) {
	presetPath, err := session.ResolvePresetPath(target)
	if err != nil {
		return "", "", "", err
	}
	preset, err := session.LoadPreset(presetPath)
	if err != nil {
		return "", "", "", err
	}
	dir := session.ExpandPath(preset.StartDirectory)
	if dir == "" {
		dir = session.CanonicalDir("")
	} else {
		dir = session.CanonicalDir(dir)
	}
	return socketPathForDir(dir), dir, presetPath, nil
}

func runDaemon(presetPath, configPath string) error {
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
		p, presetErr := session.LoadPreset(presetPath)
		if presetErr != nil {
			return presetErr
		}
		srv.SetPreset(p)
	}
	return srv.Run()
}

func runClientWithConfig(sock, dir, presetPath, configPath string) error {
	if os.Getenv("SLAT") != "" {
		return errors.New("already inside a slat session (unset SLAT to force)")
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
			if spawnErr := spawnDaemon(sock, dir, presetPath, configPath); spawnErr != nil {
				return fmt.Errorf("failed to start daemon: %w", spawnErr)
			}
			if waitErr := waitForSocket(sock, 3*time.Second); waitErr != nil {
				return fmt.Errorf("%w (see %s.log)", waitErr, sock)
			}
		}
		detached, err := client.Run(sock)
		if err != nil {
			return err
		}
		if detached {
			if resp, popErr := control.Do(sock, control.Request{Cmd: "pop-switch"}, 500*time.Millisecond); popErr == nil && resp.SwitchTo != "" {
				if exe, exErr := os.Executable(); exErr == nil {
					if execErr := execAttach(exe, resp.SwitchTo); execErr != nil {
						return execErr
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
	c, err := (&net.Dialer{Timeout: 200 * time.Millisecond}).DialContext(context.Background(), "unix", sock)
	if err != nil {
		return false
	}
	if closeErr := c.Close(); closeErr != nil {
		log.Printf("slat: close probe socket: %v", closeErr)
	}
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
	defer func() {
		if closeErr := devnull.Close(); closeErr != nil {
			log.Printf("slat: close %s: %v", os.DevNull, closeErr)
		}
	}()
	// The daemon has no terminal; its errors and any crash trace go here.
	logf, err := os.OpenFile(sock+".log", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := logf.Close(); closeErr != nil {
			log.Printf("slat: close daemon log: %v", closeErr)
		}
	}()
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
	cmd := exec.CommandContext(context.Background(), exe, daemonArgs...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, logf, logf
	cmd.SysProcAttr = detachAttrs()
	if startErr := cmd.Start(); startErr != nil {
		return startErr
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
