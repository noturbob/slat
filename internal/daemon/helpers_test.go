package daemon_test

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/control"
	"github.com/noturbob/slat/internal/daemon"
)

// session starts a daemon on a private socket, as `slat` would. tweak, if
// given, adjusts the config before the session starts.
func session(t *testing.T, tweak ...func(*config.Config)) string {
	t.Helper()
	t.Setenv("PS1", "$ ")
	t.Setenv("ENV", "")
	// …and never the user's saved session either: these daemons save on
	// shutdown, which would otherwise overwrite a real one.
	if os.Getenv("XDG_STATE_HOME") == "" {
		t.Setenv("XDG_STATE_HOME", t.TempDir())
	}
	// Short, and never the user's own socket: a test must not be able to
	// reach a real session.
	dir, err := os.MkdirTemp("", "slat")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s.sock")

	cfg := config.DefaultConfig()
	cfg.StatusBar = false
	cfg.Agent.InputAfter = config.Duration(400 * time.Millisecond)
	for _, f := range tweak {
		f(cfg)
	}
	srv, err := daemon.New(cfg, sock)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { srv.Run(); close(done) }()
	servers.Store(sock, &running{srv: srv, done: done})
	t.Cleanup(func() {
		srv.Stop()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("daemon did not stop")
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("unix", sock); err == nil {
			c.Close()
			return sock
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("daemon did not start")
	return ""
}

func do(t *testing.T, sock string, req control.Request) control.Response {
	t.Helper()
	resp, err := control.Do(sock, req, 30*time.Second)
	if err != nil {
		t.Fatalf("%s: %v", req.Cmd, err)
	}
	return resp
}

// statusEventually waits for a pane to reach a status, and says what it
// saw if it doesn't — status is a heuristic, so failures must be legible.
func statusEventually(t *testing.T, sock, pane, want string) control.Response {
	t.Helper()
	var last control.Response
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		last = do(t, sock, control.Request{Cmd: "status", Pane: pane})
		if last.Pane != nil && last.Pane.Status == want {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
	if last.Pane != nil {
		t.Fatalf("pane %s: status %q (%s), want %q", pane, last.Pane.Status, last.Pane.Reason, want)
	}
	t.Fatalf("pane %s: %v", pane, last.Error)
	return last
}

// running is a daemon a test started, so a test can also stop one on
// purpose -- the case the restore tests are about.
type running struct {
	srv  *daemon.Server
	done chan struct{}
}

var servers sync.Map // socket path -> *running

// stopSession ends a daemon the way a shutdown does and waits for it to
// finish, so whatever it writes on the way out is on disk when it returns.
func stopSession(t *testing.T, sock string) {
	t.Helper()
	v, ok := servers.Load(sock)
	if !ok {
		t.Fatalf("no daemon for %s", sock)
	}
	r := v.(*running)
	r.srv.Stop()
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not stop")
	}
}
