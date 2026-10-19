//go:build linux

package tunnel

import (
	"context"
	"golang.org/x/sys/unix"
	"net"
	"syscall"
)

// Bootstrap lookups must not recurse into a tunnel whose upstream is unresolved.
func ProtectResolver() func() {
	previous := net.DefaultResolver
	dialer := &net.Dialer{Control: func(network, address string, raw syscall.RawConn) error {
		var markErr error
		err := raw.Control(func(fd uintptr) { markErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MARK, SocketMark) })
		if err != nil {
			return err
		}
		return markErr
	}}
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, address)
	}}
	return func() { net.DefaultResolver = previous }
}
