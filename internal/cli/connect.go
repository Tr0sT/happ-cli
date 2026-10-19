package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/Tr0sT/happ-cli/internal/check"
	"github.com/Tr0sT/happ-cli/internal/config"
	"github.com/Tr0sT/happ-cli/internal/link"
	"github.com/Tr0sT/happ-cli/internal/sysproxy"
	"github.com/Tr0sT/happ-cli/internal/tunnel"
	"github.com/Tr0sT/happ-cli/internal/xray"
	"github.com/spf13/cobra"
)

type connectOpts struct {
	socksPort, httpPort                                                   int
	systemProxy, noRouting, skipFirewall, healthCheck, dnsProxy, failover bool
	checkInterval                                                         int
	checkURL                                                              string
}

func newConnectCmd(deps *Deps) *cobra.Command {
	var mode, subName string
	var o connectOpts
	cmd := &cobra.Command{
		Use: "connect [selector]", Aliases: []string{"up"},
		Short: "Connect in the foreground using the saved profile",
		Args:  cobra.MaximumNArgs(1), ValidArgsFunction: completeServerSelector(deps),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := applyConnectDefaults(cmd); err != nil {
				return err
			}
			if err := validateConnect(mode, o); err != nil {
				return err
			}
			sub, err := resolveSub(deps.Store, subName)
			if err != nil {
				return err
			}
			selector := ""
			if len(args) > 0 {
				selector = args[0]
			}
			srv, idx, err := selectedServer(sub, selector)
			if err != nil {
				if !o.failover || selector != "" || len(sub.Servers()) == 0 {
					return err
				}
				fmt.Printf("%v; failover enabled, trying remaining profiles.\n", err)
				srv, idx, err = selectServer(sub.Servers(), "")
				if err != nil {
					return err
				}
			}
			if !xray.Supported(srv.Protocol) && !o.failover {
				return fmt.Errorf("unsupported protocol %q", srv.Protocol)
			}
			if selector != "" || sub.Selected == "" {
				if err := deps.Store.Select(sub.Name, srv.Key()); err != nil {
					return err
				}
			}
			servers := sub.Servers()
			err = fmt.Errorf("no supported profiles available")
			attempts := 1
			if o.failover {
				attempts = len(servers)
				o.healthCheck = true
			}
			for n := 0; n < attempts; n++ {
				candidate := servers[(idx+n)%len(servers)]
				if !xray.Supported(candidate.Protocol) {
					continue
				}
				fmt.Printf("Profile: %s [%s], mode: %s\n", candidate.Tag, candidate.Protocol, mode)
				if mode == "proxy" {
					err = runProxy(cmd.Context(), candidate, o)
				} else {
					err = runTunnel(cmd.Context(), candidate, o, mode == "tun-direct")
				}
				if err == nil || cmd.Context().Err() != nil {
					return err
				}
				if n+1 < attempts {
					fmt.Printf("Connection failed: %v; trying next profile.\n", err)
				}
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&mode, "mode", "m", "proxy", "proxy, tun, or tun-direct (Linux)")
	cmd.Flags().StringVar(&subName, "sub", "", "subscription name (default: active)")
	cmd.Flags().IntVar(&o.socksPort, "socks", defaultSocksPort, "local SOCKS5 port")
	cmd.Flags().IntVar(&o.httpPort, "http", defaultHTTPPort, "local HTTP proxy port; 0 disables")
	cmd.Flags().BoolVar(&o.systemProxy, "system-proxy", false, "set macOS system proxy")
	cmd.Flags().BoolVar(&o.noRouting, "no-routing", false, "create TUN without routes")
	cmd.Flags().BoolVar(&o.skipFirewall, "skip-firewall", false, "leave firewall management to the OS")
	cmd.Flags().BoolVar(&o.healthCheck, "health-check", false, "exit nonzero after three consecutive failed probes")
	cmd.Flags().IntVar(&o.checkInterval, "check-interval", 60, "seconds between probes")
	cmd.Flags().StringVar(&o.checkURL, "check-url", config.Default().Connect.CheckURL, "HTTP(S) endpoint; success requires HTTP 2xx")
	cmd.Flags().BoolVar(&o.dnsProxy, "dns-proxy", true, "configure Xray DNS for share links; raw profiles keep their DNS")
	cmd.Flags().BoolVar(&o.failover, "failover", false, "try remaining profiles on failure without changing saved selection")
	_ = cmd.RegisterFlagCompletionFunc("sub", completeSubFlag(deps))
	return cmd
}

func applyConnectDefaults(cmd *cobra.Command) error {
	dir, err := storeDir()
	if err != nil {
		return err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	c := cfg.Connect
	defaults := map[string]any{"mode": c.Mode, "socks": c.SocksPort, "http": c.HTTPPort,
		"system-proxy": c.SystemProxy, "no-routing": c.NoRouting, "skip-firewall": c.SkipFirewall,
		"health-check": c.HealthCheck, "check-interval": c.CheckInterval, "check-url": c.CheckURL,
		"dns-proxy": c.DNSProxy, "failover": c.Failover}
	for name, value := range defaults {
		if !cmd.Flags().Changed(name) {
			if err := cmd.Flags().Set(name, fmt.Sprint(value)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateConnect(mode string, o connectOpts) error {
	if mode != "proxy" && mode != "tun" && mode != "tun-direct" {
		return fmt.Errorf("unknown mode %q", mode)
	}
	if o.socksPort < 1 || o.socksPort > 65535 || o.httpPort < 0 || o.httpPort > 65535 || o.socksPort == o.httpPort {
		return fmt.Errorf("invalid or duplicate proxy ports")
	}
	if o.checkInterval < 1 {
		return fmt.Errorf("check-interval must be positive")
	}
	if err := check.ValidateURL(o.checkURL); err != nil {
		return err
	}
	if mode != "proxy" && os.Geteuid() != 0 {
		return fmt.Errorf("TUN mode requires root; use the systemd service or run with doas/sudo")
	}
	if mode == "tun-direct" && runtime.GOOS != "linux" {
		return fmt.Errorf("tun-direct routing is currently implemented only on Linux")
	}
	return nil
}

func runProxy(ctx context.Context, srv *link.Server, o connectOpts) (err error) {
	cfg, err := xray.BuildConfig(srv, xray.Options{SocksPort: o.socksPort, HTTPPort: o.httpPort, DNSProxy: o.dnsProxy})
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
	if o.systemProxy {
		restore, e := sysproxy.Enable("127.0.0.1", o.socksPort, o.httpPort)
		if e != nil {
			return e
		}
		defer func() { err = errors.Join(err, restore()) }()
	}
	fmt.Printf("SOCKS5: 127.0.0.1:%d; HTTP: 127.0.0.1:%d\n", o.socksPort, o.httpPort)
	return waitConnection(ctx, o, false)
}

func runTun(ctx context.Context, srv *link.Server, o connectOpts) error {
	return runTunnel(ctx, srv, o, false)
}

func runTunnel(ctx context.Context, srv *link.Server, o connectOpts, native bool) (err error) {
	var ips []string
	mark := 0
	if runtime.GOOS == "linux" {
		mark = tunnel.SocketMark
	} else {
		if srv.RawConfig != nil {
			return fmt.Errorf("complete JSON profiles in TUN mode currently require Linux")
		}
		ips, err = resolveIPv4(srv.Address)
		if err != nil {
			return err
		}
		pinned := *srv
		if pinned.SNI == "" {
			pinned.SNI = srv.Address
		}
		pinned.Address = ips[0]
		srv = &pinned
	}
	restoreDNS := tunnel.ProtectResolver()
	defer restoreDNS()
	cfg, err := xray.BuildConfig(srv, xray.Options{SocksPort: o.socksPort, HTTPPort: o.httpPort, DNSProxy: o.dnsProxy, TUNDirect: native, SocketMark: mark})
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
	tun, err := tunnel.Start(tunnel.Options{SocksAddr: fmt.Sprintf("127.0.0.1:%d", o.socksPort), ServerIPs: ips, SkipRoutes: o.noRouting, NoFirewall: o.skipFirewall, Native: native})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, tun.Close()) }()
	fmt.Println("TUN is up. Ctrl+C stops the connection and removes its routes.")
	return waitConnection(ctx, o, runtime.GOOS == "linux" && !o.noRouting)
}

func waitConnection(ctx context.Context, o connectOpts, throughTun bool) error {
	if !o.healthCheck {
		<-ctx.Done()
		return nil
	}
	return monitor(ctx, time.Duration(o.checkInterval)*time.Second, func(ctx context.Context) error {
		_, err := check.Fetch(ctx, fmt.Sprintf("127.0.0.1:%d", o.socksPort), o.checkURL, 10*time.Second)
		if err != nil {
			return fmt.Errorf("SOCKS probe: %w", err)
		}
		if throughTun {
			_, err = check.FetchTunnel(ctx, "thapp", o.checkURL, 10*time.Second)
			if err != nil {
				return fmt.Errorf("TUN probe: %w", err)
			}
		}
		return nil
	})
}

func monitor(ctx context.Context, interval time.Duration, probe func(context.Context) error) error {
	failures := 0
	for {
		err := probe(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			failures++
			fmt.Printf("Health probe %d/3 failed: %v\n", failures, err)
			if failures >= 3 {
				return fmt.Errorf("connection unhealthy: %w", err)
			}
		} else {
			failures = 0
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

func resolveIPv4(host string) ([]string, error) {
	addrs, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, addr := range addrs {
		if ip := addr.To4(); ip != nil {
			out = append(out, ip.String())
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no IPv4 address for %q", host)
	}
	return out, nil
}
