//go:build linux

package tunnel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/Tr0sT/happ-cli/internal/firewall"
	"github.com/xjasonlyu/tun2socks/v2/engine"
	"golang.org/x/sys/unix"
)

const happTable = "20260"
const routeProtocol = "242"

type Tunnel struct{ teardown []func() error }

func lockNetwork() (*os.File, error) {
	f, err := os.OpenFile("/run/happ-cli-tun.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another happ TUN operation is running: %w", err)
	}
	return f, nil
}

func Start(opts Options) (*Tunnel, error) {
	opts.withDefaults()
	lock, err := lockNetwork()
	if err != nil {
		return nil, err
	}
	t := &Tunnel{teardown: []func() error{lock.Close}}
	rollback := func(cause error) (*Tunnel, error) { return nil, errors.Join(cause, t.Close()) }
	if err := cleanupRoutes(); err != nil {
		return rollback(err)
	}
	if !opts.Native {
		if _, err := net.InterfaceByName(opts.TunName); err == nil {
			return rollback(fmt.Errorf("interface %s already exists", opts.TunName))
		}
		engine.Insert(&engine.Key{Proxy: "socks5://" + opts.SocksAddr, Device: "tun://" + opts.TunName, LogLevel: opts.LogLevel, MTU: opts.MTU, UDPTimeout: 30 * time.Second})
		engine.Start()
		t.teardown = append(t.teardown, func() error { engine.Stop(); return nil })
	}
	if err := run("ip", "link", "set", opts.TunName, "up"); err != nil {
		return rollback(err)
	}
	if err := run("ip", "addr", "replace", opts.TunIP+"/30", "dev", opts.TunName); err != nil {
		return rollback(err)
	}
	// Loose filtering accepts replies on TUN without changing other interfaces.
	if err := os.WriteFile("/proc/sys/net/ipv4/conf/"+opts.TunName+"/rp_filter", []byte("2\n"), 0o644); err != nil {
		return rollback(err)
	}
	if opts.SkipRoutes {
		return t, nil
	}
	if !opts.NoFirewall {
		cleanup, err := firewall.Rules(opts.TunName)
		if err != nil {
			return rollback(err)
		}
		t.teardown = append(t.teardown, cleanup)
	}
	cleanup, err := installRoutes(routePlan(opts.TunName), run)
	if err != nil {
		return rollback(err)
	}
	t.teardown = append(t.teardown, cleanup)
	return t, nil
}

func routePlan(iface string) [][]string {
	var steps [][]string
	for _, family := range []string{"-4", "-6"} {
		route := func(args ...string) {
			step := append([]string{family, "route", "add"}, args...)
			steps = append(steps, append(step, "table", happTable, "proto", routeProtocol))
		}
		if family == "-4" {
			for _, cidr := range localNets {
				route("throw", cidr)
			}
			route("default", "dev", iface)
		} else {
			for _, cidr := range []string{"fc00::/7", "fe80::/10", "ff00::/8"} {
				route("throw", cidr)
			}
			route("unreachable", "default")
		}
		rule := func(priority string, args ...string) {
			step := append([]string{family, "rule", "add", "pref", priority}, args...)
			steps = append(steps, append(step, "protocol", routeProtocol))
		}
		rule("20257", "fwmark", strconv.Itoa(SocketMark), "lookup", "main")
		// Consult actual connected/static routes, preserving Docker, LAN and WARP veth.
		rule("20258", "lookup", "main", "suppress_prefixlength", "0")
		rule("20259", "ipproto", "tcp", "sport", "22", "lookup", "main")
		rule("20260", "lookup", happTable)
	}
	return steps
}

func installRoutes(steps [][]string, execute func(string, ...string) error) (func() error, error) {
	var applied [][]string
	cleanup := func() error {
		var result error
		for i := len(applied) - 1; i >= 0; i-- {
			args := append([]string(nil), applied[i]...)
			args[2] = "del"
			result = errors.Join(result, execute("ip", args...))
		}
		applied = nil
		return result
	}
	for _, args := range steps {
		if err := execute("ip", args...); err != nil {
			return nil, errors.Join(err, cleanup())
		}
		applied = append(applied, args)
	}
	return cleanup, nil
}

func (t *Tunnel) Close() error {
	if t == nil {
		return nil
	}
	var result error
	for i := len(t.teardown) - 1; i >= 0; i-- {
		result = errors.Join(result, t.teardown[i]())
	}
	t.teardown = nil
	return result
}

// Remove only our protocol-tagged rules/routes; never flush someone else's table.
func cleanupRoutes() error {
	type snapshot struct {
		family string
		rules  []map[string]any
		routes []map[string]any
	}
	var snapshots []snapshot
	for _, family := range []string{"-4", "-6"} {
		s := snapshot{family: family}
		for _, entry := range []struct {
			kind string
			dst  *[]map[string]any
		}{{"rule", &s.rules}, {"route", &s.routes}} {
			args := []string{"-N", "-j", family, entry.kind, "show"}
			if entry.kind == "route" {
				args = append(args, "table", "all")
			}
			out, err := exec.Command("ip", args...).Output()
			if err != nil {
				return fmt.Errorf("inspect %s %s: %w", family, entry.kind, err)
			}
			if err = json.Unmarshal(out, entry.dst); err != nil {
				return err
			}
		}
		for _, r := range s.rules {
			priority, _ := r["priority"].(float64)
			if priority >= 20257 && priority <= 20260 && fmt.Sprint(r["protocol"]) != routeProtocol {
				return fmt.Errorf("routing priority %.0f is occupied by another service", priority)
			}
		}
		for _, r := range s.routes {
			if fmt.Sprint(r["table"]) == happTable && fmt.Sprint(r["protocol"]) != routeProtocol {
				return fmt.Errorf("routing table %s is occupied by another service", happTable)
			}
		}
		snapshots = append(snapshots, s)
	}
	for _, s := range snapshots {
		for _, r := range s.rules {
			priority, _ := r["priority"].(float64)
			if priority >= 20257 && priority <= 20260 && fmt.Sprint(r["protocol"]) == routeProtocol {
				if err := run("ip", s.family, "rule", "del", "pref", strconv.Itoa(int(priority)), "protocol", routeProtocol); err != nil {
					return err
				}
			}
		}
		for _, r := range s.routes {
			if fmt.Sprint(r["table"]) == happTable && fmt.Sprint(r["protocol"]) == routeProtocol {
				if err := run("ip", s.family, "route", "flush", "table", happTable, "proto", routeProtocol); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

func Cleanup() error {
	lock, err := lockNetwork()
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := cleanupRoutes(); err != nil {
		return err
	}
	return firewall.Cleanup("thapp")
}
