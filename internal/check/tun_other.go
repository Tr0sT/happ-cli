//go:build !linux

package check

import (
	"context"
	"fmt"
	"time"
)

func FetchTunnel(context.Context, string, string, time.Duration) (string, error) {
	return "", fmt.Errorf("interface-bound TUN checks require Linux")
}
