//go:build linux

package firewall

import (
	"errors"
	"fmt"
	"os/exec"
)

var execute = func(args ...string) error {
	out, err := exec.Command("iptables", append([]string{"-w", "5"}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables: %w: %s", err, out)
	}
	return nil
}

func rules(iface string) (func() error, error) {
	// Delete only tagged leftovers from an interrupted previous run.
	if err := Cleanup(iface); err != nil {
		return nil, err
	}
	var applied [][]string
	cleanup := func() error {
		var result error
		for i := len(applied) - 1; i >= 0; i-- {
			result = errors.Join(result, execute(append([]string{"-D"}, applied[i]...)...))
		}
		applied = nil
		return result
	}
	for _, direction := range []string{"-i", "-o"} {
		rule := []string{"FORWARD", direction, iface, "-m", "comment", "--comment", "happ-cli", "-j", "ACCEPT"}
		if err := execute(append([]string{"-I"}, rule...)...); err != nil {
			return nil, errors.Join(err, cleanup())
		}
		applied = append(applied, rule)
	}
	return cleanup, nil
}

func Cleanup(iface string) error {
	// A failed table listing is a real error, not evidence that no rule exists.
	if err := execute("-S", "FORWARD"); err != nil {
		return err
	}
	for _, direction := range []string{"-i", "-o"} {
		rule := []string{"FORWARD", direction, iface, "-m", "comment", "--comment", "happ-cli", "-j", "ACCEPT"}
		for execute(append([]string{"-C"}, rule...)...) == nil {
			if err := execute(append([]string{"-D"}, rule...)...); err != nil {
				return err
			}
		}
	}
	return nil
}
