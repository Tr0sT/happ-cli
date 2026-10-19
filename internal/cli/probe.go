package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tr0sT/happ-cli/internal/check"
	"github.com/Tr0sT/happ-cli/internal/link"
	"github.com/Tr0sT/happ-cli/internal/store"
	"github.com/Tr0sT/happ-cli/internal/xray"
	"net"
	"time"
)

func selectWorking(ctx context.Context, st Store, sub store.SubEntry, target string) error {
	if err := check.ValidateURL(target); err != nil {
		return err
	}
	for i, srv := range sub.Servers() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !xray.Supported(srv.Protocol) {
			continue
		}
		started := time.Now()
		err := probeServer(ctx, srv, target)
		if err != nil {
			fmt.Printf("#%d %s: failed (%v)\n", i+1, srv.Tag, err)
			continue
		}
		if err := st.Select(sub.Name, srv.Key()); err != nil {
			return err
		}
		fmt.Printf("Selected #%d %s: HTTPS/HTTP OK in %s. Restart the connection to apply.\n", i+1, srv.Tag, time.Since(started).Round(time.Millisecond))
		return nil
	}
	return fmt.Errorf("no profile passed the probe; saved selection unchanged")
}

func probeServer(ctx context.Context, srv *link.Server, target string) (err error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	cfg, err := xray.BuildConfig(srv, xray.Options{SocksPort: port, DNSProxy: true, LogLevel: "error"})
	if err != nil {
		return err
	}
	raw, err := cfg.JSON()
	if err != nil {
		return err
	}
	inst, err := xray.Start(raw)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, inst.Close()) }()
	_, err = check.Fetch(ctx, fmt.Sprintf("127.0.0.1:%d", port), target, 8*time.Second)
	return err
}
