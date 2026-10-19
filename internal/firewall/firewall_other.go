//go:build !linux

package firewall

func rules(string) (func() error, error) { return func() error { return nil }, nil }
func Cleanup(string) error               { return nil }
