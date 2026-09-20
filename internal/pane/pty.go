package pane

import "io"

// ptyProcess is a program running on a pseudo terminal. Unix and Windows
// arrive here by completely different routes — a PTY pair, a session and a
// process group, versus ConPTY, pipes and a job object — so the rest of
// the package talks to this and never to either platform directly.
//
// Read gives the program's output; Write sends it input.
type ptyProcess interface {
	io.ReadWriteCloser

	// Resize tells the program its terminal is now this big.
	Resize(rows, cols int) error
	// Pid is the program slat started.
	Pid() int
	// Wait returns once the program has exited.
	Wait()
	// Terminate ends the program and everything it started, the way
	// closing a terminal window does.
	Terminate()
	// Foreground is the process the terminal is giving input to, which is
	// how a running command is told from an idle shell. 0, "" where the
	// platform won't say (Windows has no foreground process group).
	Foreground() (pid int, name string)
	// Cwd is the program's working directory, or "" where the platform
	// won't say.
	Cwd() string
}
