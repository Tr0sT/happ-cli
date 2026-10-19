package xray

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"github.com/Tr0sT/happ-cli/internal/link"
)

// TestProxyEndToEnd stands up a real xray Shadowsocks server, a client built
// from a link.Server via BuildConfig, and verifies an HTTP request routed
// through the client's SOCKS inbound reaches a target server through the proxy.
func TestProxyEndToEnd(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "pong")
	}))
	defer target.Close()

	const method, password = "aes-256-gcm", "e2e-secret"
	ssPort := freePort(t)

	serverCfg := fmt.Sprintf(`{
      "log": {"loglevel": "warning"},
      "inbounds": [{
        "tag": "ss-in", "listen": "127.0.0.1", "port": %d, "protocol": "shadowsocks",
        "settings": {"method": %q, "password": %q, "network": "tcp"}
      }],
      "outbounds": [{"protocol": "freedom"}]
    }`, ssPort, method, password)

	server, err := Start([]byte(serverCfg))
	if err != nil {
		t.Fatalf("start ss server: %v", err)
	}
	defer server.Close()

	clientSrv := &link.Server{
		Protocol: "shadowsocks", Address: "127.0.0.1", Port: ssPort,
		Method: method, Password: password, Network: "tcp",
	}
	socksPort := freePort(t)
	cfg, err := BuildConfig(clientSrv, Options{SocksPort: socksPort})
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	raw, err := cfg.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	client, err := Start(raw)
	if err != nil {
		t.Fatalf("start client: %v\n%s", err, raw)
	}
	defer client.Close()

	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", socksPort), nil, proxy.Direct)
	if err != nil {
		t.Fatalf("socks dialer: %v", err)
	}
	httpClient := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{Dial: dialer.Dial},
	}

	// Retry briefly while both instances finish binding.
	var body string
	for i := 0; i < 20; i++ {
		resp, err := httpClient.Get(target.URL)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			body = string(b)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if body != "pong" {
		t.Fatalf("response through proxy = %q, want %q", body, "pong")
	}
}

// A complete profile whose default outbound blocks traffic proves that its
// routing/balancer, rather than an accidental direct fallback, carries HTTP.
func TestRawBalancerEndToEnd(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "raw-profile-ok") }))
	defer target.Close()
	port := freePort(t)
	server, err := Start([]byte(fmt.Sprintf(`{"inbounds":[{"listen":"127.0.0.1","port":%d,"protocol":"shadowsocks","settings":{"method":"aes-256-gcm","password":"test-secret","network":"tcp"}}],"outbounds":[{"protocol":"freedom"}]}`, port)))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	raw := fmt.Sprintf(`{"remarks":"AUTO","outbounds":[{"tag":"block","protocol":"blackhole"},{"tag":"p","protocol":"shadowsocks","settings":{"servers":[{"address":"127.0.0.1","port":%d,"method":"aes-256-gcm","password":"test-secret"}]}}],"routing":{"balancers":[{"tag":"auto","selector":["p"]}],"rules":[{"type":"field","network":"tcp,udp","balancerTag":"auto"}]}}`, port)
	srv, err := link.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	socks := freePort(t)
	cfg, err := BuildConfig(srv, Options{SocksPort: socks})
	if err != nil {
		t.Fatal(err)
	}
	config, err := cfg.JSON()
	if err != nil {
		t.Fatal(err)
	}
	client, err := Start(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", socks), nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Dial: dialer.Dial}
	defer transport.CloseIdleConnections()
	resp, err := (&http.Client{Transport: transport, Timeout: 3 * time.Second}).Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "raw-profile-ok" {
		t.Fatalf("routing failed: %s %v", body, err)
	}
}
