//go:build !linux

package tunnel

import "fmt"

func ProtectResolver() func() { return func() {} }
func Cleanup() error          { return fmt.Errorf("cleanup-tun is only implemented on Linux") }
