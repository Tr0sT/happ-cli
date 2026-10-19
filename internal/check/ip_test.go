package check

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeRejectsFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503); w.Write([]byte("ip=1.2.3.4")) }))
	defer server.Close()
	_, err := fetch(context.Background(), server.URL, time.Second, (&net.Dialer{}).DialContext)
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("unhealthy endpoint accepted: %v", err)
	}
}

func TestProbeParsesTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("fl=123\nip=1.2.3.4\n")) }))
	defer server.Close()
	ip, err := fetch(context.Background(), server.URL, time.Second, (&net.Dialer{}).DialContext)
	if err != nil || ip != "1.2.3.4" {
		t.Fatalf("ip=%q err=%v", ip, err)
	}
}

func TestSOCKSFailureNeverFallsBackToDirect(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.WriteHeader(200) }))
	defer server.Close()
	l, _ := net.Listen("tcp4", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	if _, err := Fetch(context.Background(), addr, server.URL, time.Second); err == nil {
		t.Fatal("dead proxy accepted")
	}
	if hits != 0 {
		t.Fatal("direct fallback leaked request")
	}
}
