//go:build linux

package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Tr0sT/happ-cli/internal/check"
	"github.com/Tr0sT/happ-cli/internal/link"
	"github.com/Tr0sT/happ-cli/internal/xray"
)

func TestNetworkPeer(t *testing.T) {
	if os.Getenv("HAPP_NETWORK_PEER") != "1" {
		t.Skip("network namespace helper")
	}
	instance, err := xray.Start([]byte(`{"log":{"loglevel":"error"},"inbounds":[{"listen":"203.0.113.1","port":18443,"protocol":"shadowsocks","settings":{"method":"aes-256-gcm","password":"netns-test","network":"tcp,udp"}}],"outbounds":[{"protocol":"freedom"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	err = http.ListenAndServe("203.0.113.2:18080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "netns-ok") }))
	t.Fatal(err)
}

// Run only inside an isolated user + mount + network namespace (tests/netns.sh).
func TestLinuxTunnelIntegration(t *testing.T) {
	if os.Getenv("HAPP_NETNS_TEST") != "1" {
		t.Skip("requires isolated network namespace")
	}
	ip := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("ip", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("ip %v: %v %s", args, err, out)
		}
		return string(out)
	}
	ip("link", "set", "lo", "up")
	ip("netns", "add", "happ-test-peer")
	t.Cleanup(func() { exec.Command("ip", "netns", "del", "happ-test-peer").Run() })
	ip("link", "add", "wan", "type", "veth", "peer", "name", "peer")
	ip("link", "set", "peer", "netns", "happ-test-peer")
	ip("addr", "add", "192.0.2.1/30", "dev", "wan")
	ip("link", "set", "wan", "up")
	ip("-n", "happ-test-peer", "addr", "add", "192.0.2.2/30", "dev", "peer")
	ip("-n", "happ-test-peer", "link", "set", "peer", "up")
	ip("-n", "happ-test-peer", "link", "set", "lo", "up")
	for _, addr := range []string{"203.0.113.1/32", "203.0.113.2/32"} {
		ip("-n", "happ-test-peer", "addr", "add", addr, "dev", "lo")
	}
	ip("route", "add", "default", "via", "192.0.2.2", "dev", "wan")
	for name, addr := range map[string]string{"docker-test": "172.18.0.1/16", "cf-warp-test": "10.254.253.1/30"} {
		ip("link", "add", name, "type", "dummy")
		ip("addr", "add", addr, "dev", name)
		ip("link", "set", name, "up")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	peer := exec.CommandContext(ctx, "ip", "netns", "exec", "happ-test-peer", binary, "-test.run=^TestNetworkPeer$")
	peer.Env = append(os.Environ(), "HAPP_NETWORK_PEER=1")
	peer.Stdout, peer.Stderr = os.Stdout, os.Stderr
	if err := peer.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); peer.Wait() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp4", "203.0.113.2:18080", 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("peer did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%v", native), func(t *testing.T) {
			srv, err := link.Parse(`{"remarks":"test profile","outbounds":[{"tag":"proxy","protocol":"shadowsocks","settings":{"servers":[{"address":"203.0.113.1","port":18443,"method":"aes-256-gcm","password":"netns-test"}]}}]}`)
			if err != nil {
				t.Fatal(err)
			}
			runCtx, stop := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() { done <- runTunnel(runCtx, srv, connectOpts{socksPort: 10808, httpPort: 0}, native) }()
			stopped := false
			defer func() {
				if !stopped {
					stop()
					<-done
				}
			}()
			deadline := time.Now().Add(5 * time.Second)
			for {
				select {
				case err := <-done:
					stopped = true
					t.Fatalf("TUN exited early: %v", err)
				default:
				}
				out, _ := exec.Command("ip", "-4", "route", "get", "203.0.113.2").Output()
				if strings.Contains(string(out), "dev thapp") {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("TUN routing missing: %s", out)
				}
				time.Sleep(50 * time.Millisecond)
			}
			for dest, iface := range map[string]string{"172.18.0.2": "docker-test", "10.254.253.2": "cf-warp-test", "192.0.2.2": "wan"} {
				if route := ip("-4", "route", "get", dest); !strings.Contains(route, "dev "+iface) {
					t.Fatalf("local route broken: %s", route)
				}
			}
			if route := ip("-4", "route", "get", "203.0.113.2", "from", "10.254.253.2", "iif", "cf-warp-test"); !strings.Contains(route, "dev thapp") {
				t.Fatalf("forwarded WARP traffic bypasses TUN: %s", route)
			}
			body, err := check.FetchTunnel(ctx, "thapp", "http://203.0.113.2:18080", 5*time.Second)
			if err != nil || body != "netns-ok" {
				t.Fatalf("TUN request failed: %q %v", body, err)
			}
			if out, err := exec.Command("ip", "-6", "route", "get", "2606:4700:4700::1111").CombinedOutput(); err == nil {
				t.Fatalf("IPv6 leaked: %s", out)
			}
			stop()
			err = <-done
			stopped = true
			if err != nil {
				t.Fatal(err)
			}
			if route := ip("-4", "route", "get", "203.0.113.2"); !strings.Contains(route, "dev wan") {
				t.Fatalf("route was not restored: %s", route)
			}
			for _, family := range []string{"-4", "-6"} {
				if rules := ip("-j", family, "rule", "show"); strings.Contains(rules, "20260") {
					t.Fatalf("stale rules: %s", rules)
				}
			}
		})
	}
}
