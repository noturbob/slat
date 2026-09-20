package daemon

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/noturbob/slat/internal/app"
	"github.com/noturbob/slat/internal/control"
	"github.com/noturbob/slat/internal/proto"
)

// waitCap bounds how long a single `slat wait` may block the connection.
const waitCap = 24 * time.Hour

// serveControl answers one CLI command and closes the connection. It
// never attaches the connection to the session, so an agent running
// commands doesn't disconnect whoever is watching the panes.
func (s *Server) serveControl(conn net.Conn, payload []byte) {
	var req control.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		s.reply(conn, control.Errorf(control.CodeError, "bad request: %v", err))
		return
	}
	s.reply(conn, s.run(req))
}

func (s *Server) run(req control.Request) control.Response {
	a := s.app
	pane := req.Pane
	if pane == "" {
		pane = "active"
	}

	switch req.Cmd {
	case "ls":
		return control.Response{Schema: control.Schema, Panes: a.Panes()}

	case "status":
		if req.Pane == "" {
			return control.Response{Schema: control.Schema, Panes: a.Panes()}
		}
		info, err := a.PaneInfo(pane)
		if err != nil {
			return paneErr(err)
		}
		return control.Response{Schema: control.Schema, Pane: &info}

	case "pane-new":
		info, err := a.NewPane(app.NewPaneOpts{
			Target: req.Pane, Split: req.Split, Cwd: req.Cwd,
			Cmd: req.Command, Focus: req.Focus,
		})
		if err != nil {
			return control.Errorf(control.CodeError, "%v", err)
		}
		return control.Response{Schema: control.Schema, Pane: &info}

	case "pane-close":
		if err := a.ClosePane(pane); err != nil {
			return paneErr(err)
		}
		return control.Response{Schema: control.Schema}

	case "send":
		if err := a.Send(pane, []byte(req.Data)); err != nil {
			return paneErr(err)
		}
		return control.Response{Schema: control.Schema}

	case "capture":
		lines, err := a.Capture(pane, req.Lines, req.History)
		if err != nil {
			return paneErr(err)
		}
		return control.Response{Schema: control.Schema, Lines: lines}

	case "wait":
		return s.wait(req, pane)
	}
	return control.Errorf(control.CodeError, "unknown command %q", req.Cmd)
}

func (s *Server) wait(req control.Request, pane string) control.Response {
	cond, err := app.ParseWaitFor(req.For)
	if err != nil {
		return control.Errorf(control.CodeError, "%v", err)
	}
	timeout := time.Minute
	if req.Timeout != "" {
		d, err := time.ParseDuration(req.Timeout)
		if err != nil || d < 0 {
			return control.Errorf(control.CodeError, "bad --timeout %q", req.Timeout)
		}
		timeout = d
	}
	if timeout == 0 || timeout > waitCap {
		timeout = waitCap
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	info, met, err := s.app.Wait(ctx, pane, cond)
	if err != nil {
		if cond.Exit {
			// The pane left the session: that is the exit we waited for.
			met = true
		} else {
			return control.Errorf(control.CodeError, "%v", err)
		}
	}
	resp := control.Response{Schema: control.Schema, Matched: &met, Pane: &info}
	if !met {
		resp.Code = control.CodeTimeout
		resp.Error = "timed out after " + timeout.String()
	}
	return resp
}

// paneErr reports a pane that has closed with its own exit code, so an
// agent can tell "that pane is finished" from "that command was wrong".
func paneErr(err error) control.Response {
	if app.PaneGone(err) {
		return control.Errorf(control.CodePaneGone, "%v", err)
	}
	return control.Errorf(control.CodeError, "%v", err)
}

func (s *Server) reply(conn net.Conn, resp control.Response) {
	resp.Schema = control.Schema
	body, err := json.Marshal(resp)
	if err != nil {
		body = []byte(`{"schema":1,"error":"could not encode response"}`)
	}
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	proto.WriteFrame(conn, proto.TypeControl, body)
}
