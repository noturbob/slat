package daemon

import "net"

// listenPrivate creates the socket. Windows has no umask: an AF_UNIX
// socket there is a file in the user's own temporary directory and
// inherits its permissions.
func listenPrivate(path string) (net.Listener, error) {
	return net.Listen("unix", path)
}
