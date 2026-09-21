// Package control is the wire format between the slat CLI and the
// daemon: one JSON request, one JSON response, over the daemon's socket.
// See docs/design/agent-cli.md.
package control

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/noturbob/slat/internal/app"
	"github.com/noturbob/slat/internal/proto"
)

// Schema is the response format version.
const Schema = 1

// Exit codes, shared by the CLI and documented for agents.
const (
	CodeOK       = 0
	CodeError    = 1
	CodeTimeout  = 2
	CodePaneGone = 3
)

// Request is one command.
type Request struct {
	Cmd     string `json:"cmd"`
	Pane    string `json:"pane,omitempty"`
	Data    string `json:"data,omitempty"`    // send: bytes to write
	Split   string `json:"split,omitempty"`   // pane new: v or h
	Cwd     string `json:"cwd,omitempty"`     // pane new
	Command string `json:"command,omitempty"` // pane new: run this
	Focus   bool   `json:"focus,omitempty"`   // pane new: keep focus there
	Lines   int    `json:"lines,omitempty"`   // capture
	History bool   `json:"history,omitempty"` // capture
	For     string `json:"for,omitempty"`     // wait
	Timeout string `json:"timeout,omitempty"` // wait
}

// Response is the answer. Exactly one of Error, Panes, Pane or Lines is
// normally set; Code carries the CLI's exit code.
type Response struct {
	Schema  int            `json:"schema"`
	Code    int            `json:"code,omitempty"`
	Error   string         `json:"error,omitempty"`
	Panes   []app.PaneInfo `json:"panes,omitempty"`
	Pane    *app.PaneInfo  `json:"pane,omitempty"`
	Lines   []string       `json:"lines,omitempty"`
	Matched *bool          `json:"matched,omitempty"` // wait: did the condition hold
}

// Errorf builds a failed response.
func Errorf(code int, format string, args ...any) Response {
	return Response{Schema: Schema, Code: code, Error: fmt.Sprintf(format, args...)}
}

// Do sends one request to the daemon at sock and returns its response.
// Control connections never attach to the session, so they don't disturb
// whoever is using it.
func Do(sock string, req Request, timeout time.Duration) (Response, error) {
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return Response{}, fmt.Errorf("no slat session (%w)", err)
	}
	defer conn.Close()

	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	if err := proto.WriteFrame(conn, proto.TypeControl, body); err != nil {
		return Response{}, err
	}
	if timeout > 0 {
		conn.SetReadDeadline(time.Now().Add(timeout))
	}
	frame, err := proto.ReadFrame(conn)
	if err != nil {
		return Response{}, fmt.Errorf("daemon closed the connection: %w", err)
	}
	var resp Response
	if err := json.Unmarshal(frame.Payload, &resp); err != nil {
		return Response{}, fmt.Errorf("bad response from daemon: %w", err)
	}
	if resp.Error != "" && resp.Code == 0 {
		resp.Code = CodeError
	}
	return resp, nil
}
