//go:build linux

package firewall

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestPartialFailureRemovesOnlyOwnInstalledRule(t *testing.T) {
	saved := execute
	defer func() { execute = saved }()
	var deleted []string
	execute = func(args ...string) error {
		line := strings.Join(args, " ")
		if args[0] == "-C" {
			return fmt.Errorf("not found")
		}
		if args[0] == "-I" && strings.Contains(line, "-o thapp") {
			return fmt.Errorf("injected failure")
		}
		if args[0] == "-D" {
			deleted = append(deleted, line)
		}
		return nil
	}
	if _, err := Rules("thapp"); err == nil {
		t.Fatal("expected failure")
	}
	want := []string{"-D FORWARD -i thapp -m comment --comment happ-cli -j ACCEPT"}
	if !reflect.DeepEqual(deleted, want) {
		t.Fatalf("deleted %v", deleted)
	}
}
