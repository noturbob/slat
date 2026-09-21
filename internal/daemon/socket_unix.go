//go:build !windows

package daemon

import (
	"net"
	"syscall"
)

// listenPrivate creates the socket with mode 0600, so only its user can
// attach to the session.
func listenPrivate(path string) (net.Listener, error) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	return net.Listen("unix", path)
}
