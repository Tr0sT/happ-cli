package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tr0sT/happ-cli/internal/link"
	"github.com/Tr0sT/happ-cli/internal/store"
	"github.com/Tr0sT/happ-cli/internal/xray"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHomeIsParsedBeforeOpeningStore(t *testing.T) {
	previous := homeDir
	defer func() { homeDir = previous }()
	dir := t.TempDir()
	st, _ := store.Open(dir)
	if err := st.Upsert(store.SubEntry{Name: "custom", Links: []string{testSSServer().Raw}}); err != nil {
		t.Fatal(err)
	}
	deps := DefaultDeps(nil)
	cmd := newRootCmd("test", deps)
	cmd.SetArgs([]string{"--home", dir, "sub", "use", "custom"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if deps.Store.Active() != "custom" {
		t.Fatal("--home was ignored")
	}
}

func TestDefaultsAndCLIOverrides(t *testing.T) {
	previous := homeDir
	defer func() { homeDir = previous }()
	homeDir = t.TempDir()
	os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte("connect:\n  socks-port: 12000\n  health-check: true\n"), 0o600)
	cmd := newConnectCmd(withDeps())
	cmd.ParseFlags([]string{"--socks", "12001"})
	if err := applyConnectDefaults(cmd); err != nil {
		t.Fatal(err)
	}
	port, _ := cmd.Flags().GetInt("socks")
	health, _ := cmd.Flags().GetBool("health-check")
	http, _ := cmd.Flags().GetInt("http")
	if port != 12001 || !health || http != 10809 {
		t.Fatalf("defaults/override lost: %d %v %d", port, health, http)
	}
}

func TestSelectionSurvivesReorderingAndMissingIsExplicit(t *testing.T) {
	a, _ := link.Parse("trojan://pw@a.example:443#A")
	b, _ := link.Parse("trojan://pw@b.example:443#B")
	sub := store.SubEntry{Name: "vpn", Links: []string{b.Raw, a.Raw}, Selected: a.Key()}
	selected, index, err := selectedServer(sub, "")
	if err != nil || index != 1 || selected.Tag != "A" {
		t.Fatalf("selection changed: %v", err)
	}
	sub.Links = []string{b.Raw}
	if _, _, err := selectedServer(sub, ""); err == nil {
		t.Fatal("missing selection silently fell back")
	}
	if _, _, err := selectServer([]*link.Server{{Tag: "AUTO one"}, {Tag: "AUTO two"}}, "AUTO"); err == nil {
		t.Fatal("ambiguous match accepted")
	}
}

func TestHealthRequiresConsecutiveFailuresAndReturnsError(t *testing.T) {
	calls := 0
	err := monitor(context.Background(), time.Nanosecond, func(context.Context) error {
		calls++
		if calls == 3 {
			return nil
		}
		return errors.New("offline")
	})
	if err == nil || calls != 6 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := monitor(ctx, time.Nanosecond, func(context.Context) error { return ctx.Err() }); err != nil {
		t.Fatalf("normal stop reported failure: %v", err)
	}
}

func TestWorkingSelectionDoesNotDestroyPreferenceOnFailure(t *testing.T) {
	st := &mockStore{subs: []store.SubEntry{{Name: "vpn", Selected: "keep", Links: []string{`hysteria2://pw@host:443#unsupported`}}}}
	if err := selectWorking(context.Background(), st, st.subs[0], "https://api.github.com"); err == nil {
		t.Fatal("expected no working server")
	}
	if st.subs[0].Selected != "keep" {
		t.Fatal("preference changed")
	}
}

func TestSelectWorkingSkipsDeadProxyAndPersistsLiveOne(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer target.Close()
	free := func() int {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		p := l.Addr().(*net.TCPAddr).Port
		l.Close()
		return p
	}
	livePort := free()
	deadPort := free()
	server, err := xray.Start([]byte(fmt.Sprintf(`{"inbounds":[{"listen":"127.0.0.1","port":%d,"protocol":"shadowsocks","settings":{"method":"aes-256-gcm","password":"password","network":"tcp"}}],"outbounds":[{"protocol":"freedom"}]}`, livePort)))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	links := []string{fmt.Sprintf("ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ=@127.0.0.1:%d#Dead", deadPort), fmt.Sprintf("ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ=@127.0.0.1:%d#Live", livePort)}
	if err := st.Upsert(store.SubEntry{Name: "vpn", Links: links}); err != nil {
		t.Fatal(err)
	}
	sub, _ := st.Find("vpn")
	if err := selectWorking(context.Background(), st, sub, target.URL); err != nil {
		t.Fatal(err)
	}
	saved, _ := st.Find("vpn")
	live, _ := link.Parse(links[1])
	if saved.Selected != live.Key() {
		t.Fatal("did not retain the working profile")
	}
}
