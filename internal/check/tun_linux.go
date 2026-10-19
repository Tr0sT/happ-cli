//go:build linux

package check

import (
	"context"
	"golang.org/x/sys/unix"
	"net"
	"syscall"
	"time"
)

func FetchTunnel(ctx context.Context, iface, target string, timeout time.Duration) (string, error) {
	dialer := &net.Dialer{Timeout: timeout, Control: func(network, address string, raw syscall.RawConn) error {
		var bindErr error
		err := raw.Control(func(fd uintptr) { bindErr = unix.BindToDevice(int(fd), iface) })
		if err != nil {
			return err
		}
		return bindErr
	}}
	return fetch(ctx, target, timeout, func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp4", address)
	})
}
