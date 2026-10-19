//go:build linux

package tunnel

import (
	"fmt"
	"strings"
	"testing"
)

func TestRoutePlanPreservesMainAndBlocksIPv6(t *testing.T) {
	var lines []string
	for _, args := range routePlan("thapp") {
		lines = append(lines, strings.Join(args, " "))
	}
	plan := strings.Join(lines, "\n")
	for _, required := range []string{
		"-4 rule add pref 20258 lookup main suppress_prefixlength 0",
		"-4 rule add pref 20259 ipproto tcp sport 22 lookup main",
		"-4 route add throw 10.0.0.0/8",
		"-4 route add throw 172.16.0.0/12",
		"-4 route add default dev thapp",
		"-6 route add unreachable default",
		"-6 rule add pref 20260 lookup 20260",
	} {
		if !strings.Contains(plan, required) {
			t.Errorf("missing %s", required)
		}
	}
	if strings.Contains(plan, " via ") {
		t.Fatal("private networks must use their actual main-table routes")
	}
}

func TestRouteFailureRollsBackExactlyAppliedSteps(t *testing.T) {
	for failAt := 0; failAt < len(routePlan("thapp")); failAt++ {
		var installed [][]string
		count := 0
		execute := func(name string, args ...string) error {
			if args[2] == "add" {
				if count == failAt {
					return fmt.Errorf("injected")
				}
				count++
				installed = append(installed, append([]string(nil), args...))
			} else {
				if len(installed) == 0 {
					t.Fatal("removed unowned route")
				}
				want := installed[len(installed)-1]
				want[2] = "del"
				if strings.Join(args, " ") != strings.Join(want, " ") {
					t.Fatalf("wrong rollback: %v", args)
				}
				installed = installed[:len(installed)-1]
			}
			return nil
		}
		if _, err := installRoutes(routePlan("thapp"), execute); err == nil {
			t.Fatal("expected failure")
		}
		if len(installed) != 0 {
			t.Fatalf("routes remain after failure at %d", failAt)
		}
	}
}
