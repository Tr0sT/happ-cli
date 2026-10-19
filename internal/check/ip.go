package check

import (
	"context"
	"fmt"
	"golang.org/x/net/proxy"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("check URL must be an absolute HTTP(S) URL")
	}
	return nil
}

// Fetch never falls back to a direct connection when SOCKS is unavailable.
func Fetch(ctx context.Context, socksAddr, target string, timeout time.Duration) (string, error) {
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, &net.Dialer{Timeout: timeout})
	if err != nil {
		return "", err
	}
	return fetch(ctx, target, timeout, dialer.(proxy.ContextDialer).DialContext)
}

func fetch(ctx context.Context, target string, timeout time.Duration, dial func(context.Context, string, string) (net.Conn, error)) (string, error) {
	if err := ValidateURL(target); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	transport := &http.Transport{DialContext: dial}
	defer transport.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Transport: transport, Timeout: timeout}).Do(req)
	if err != nil {
		if e, ok := err.(*url.Error); ok {
			err = e.Err
		}
		return "", fmt.Errorf("HTTP probe: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		if value, ok := strings.CutPrefix(line, "ip="); ok {
			return strings.TrimSpace(value), nil
		}
	}
	return strings.TrimSpace(string(body)), nil
}

func ExternalIP(addr string, timeout time.Duration) (string, error) {
	return Fetch(context.Background(), addr, "https://cloudflare.com/cdn-cgi/trace", timeout)
}
func FetchURL(addr, target string, timeout time.Duration) (string, error) {
	return Fetch(context.Background(), addr, target, timeout)
}
func PrintIP(port int) {
	ip, err := ExternalIP(fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second)
	if err != nil {
		fmt.Printf("IP check: %v\n", err)
		return
	}
	fmt.Printf("Exit IP: %s\n", ip)
}
