// Package control is the wire format between the slat CLI and the
// daemon: one JSON request, one JSON response, over the daemon's socket.
// See docs/design/agent-cli.md.
package control

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"path/filepath"
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
	For             string `json:"for,omitempty"`              // wait
	Timeout         string `json:"timeout,omitempty"`          // wait
	TargetWorkspace string `json:"target_workspace,omitempty"` // workspace name or 1-based index
	TargetTab       string `json:"target_tab,omitempty"`       // tab name or 1-based index
	Direction       string `json:"direction,omitempty"`        // focus: left, down, up, right
	Workspace       int    `json:"workspace,omitempty"`
	Tab             int    `json:"tab,omitempty"`
	PaneID          int    `json:"pane_id,omitempty"`
}

// Response is the answer. Exactly one of Error, Panes, Pane or Lines is
// normally set; Code carries the CLI's exit code.
type Response struct {
	Schema   int                  `json:"schema"`
	Code     int                  `json:"code,omitempty"`
	Error    string               `json:"error,omitempty"`
	Panes    []app.PaneInfo       `json:"panes,omitempty"`
	Pane     *app.PaneInfo        `json:"pane,omitempty"`
	Session  *app.SessionSummary  `json:"session,omitempty"`
	Sessions []app.SessionSummary `json:"sessions,omitempty"`
	Tree     []app.RemoteTreeNode `json:"tree,omitempty"`
	SwitchTo string               `json:"switch_to,omitempty"`
	Lines    []string             `json:"lines,omitempty"`
	Matched  *bool                `json:"matched,omitempty"` // wait: did the condition hold
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
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("slat: close control socket: %v", closeErr)
		}
	}()

	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	if writeErr := proto.WriteFrame(conn, proto.TypeControl, body); writeErr != nil {
		return Response{}, writeErr
	}
	if timeout > 0 {
		if deadlineErr := conn.SetReadDeadline(time.Now().Add(timeout)); deadlineErr != nil {
			return Response{}, deadlineErr
		}
	}
	frame, err := proto.ReadFrame(conn)
	if err != nil {
		return Response{}, fmt.Errorf("daemon closed the connection: %w", err)
	}
	var resp Response
	if unmarshalErr := json.Unmarshal(frame.Payload, &resp); unmarshalErr != nil {
		return Response{}, fmt.Errorf("bad response from daemon: %w", unmarshalErr)
	}
	if resp.Error != "" && resp.Code == 0 {
		resp.Code = CodeError
	}
	return resp, nil
}

// ListSessions discovers live slat daemons matching prefix*.sock across dirs,
// skipping any socket in skip.
func ListSessions(dirs []string, prefix string, skip ...string) []app.SessionSummary {
	seen := map[string]bool{}
	for _, s := range skip {
		if s != "" {
			seen[s] = true
		}
	}
	var out []app.SessionSummary
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(dir, prefix+"*.sock"))
		for _, sock := range matches {
			if seen[sock] {
				continue
			}
			seen[sock] = true
			resp, err := Do(sock, Request{Cmd: "session"}, 500*time.Millisecond)
			if err != nil || resp.Session == nil {
				continue
			}
			s := *resp.Session
			s.Socket = sock
			out = append(out, s)
		}
	}
	return out
}

// ListGlobalTree collects tree nodes from all live daemons matching prefix*.sock
// across dirs, skipping any socket in skip.
func ListGlobalTree(dirs []string, prefix string, skip ...string) []app.RemoteTreeNode {
	seen := map[string]bool{}
	for _, s := range skip {
		if s != "" {
			seen[s] = true
		}
	}
	var out []app.RemoteTreeNode
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(dir, prefix+"*.sock"))
		for _, sock := range matches {
			if seen[sock] {
				continue
			}
			seen[sock] = true
			resp, err := Do(sock, Request{Cmd: "tree"}, 500*time.Millisecond)
			if err != nil {
				continue
			}
			for _, n := range resp.Tree {
				n.Socket = sock
				out = append(out, n)
			}
		}
	}
	return out
}

// FocusRemote asks the daemon at sock to focus the given workspace/tab/pane.
func FocusRemote(sock string, wsIdx, tabIdx, paneID int) {
	_, _ = Do(sock, Request{
		Cmd:       "focus",
		Workspace: wsIdx,
		Tab:       tabIdx,
		PaneID:    paneID,
	}, 500*time.Millisecond)
}
