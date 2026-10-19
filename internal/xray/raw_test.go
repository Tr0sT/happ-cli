package xray

import (
	"encoding/json"
	"github.com/Tr0sT/happ-cli/internal/link"
	"reflect"
	"testing"
)

func TestRawConfigRetainsRoutingDNSAndTransport(t *testing.T) {
	raw := `{"remarks":"AUTO","log":{"access":"/tmp/provider-log"},"api":{"listen":"0.0.0.0:9999"},"inbounds":[{"tag":"provider-socks","protocol":"socks","listen":"0.0.0.0","port":9999}],"dns":{"servers":["1.1.1.1"]},"outbounds":[{"tag":"p","protocol":"trojan","settings":{"servers":[{"address":"example.com","port":443,"password":"test"}]},"streamSettings":{"network":"xhttp","xhttpSettings":{"path":"/test"},"sockopt":{"tcpFastOpen":true}}}],"routing":{"balancers":[{"tag":"auto","selector":["p"]}],"rules":[{"type":"field","inboundTag":["provider-socks"],"balancerTag":"auto"}]},"burstObservatory":{"subjectSelector":["p"]}}`
	s, err := link.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := BuildConfig(s, Options{SocksPort: 10808, TUNDirect: true, SocketMark: 123})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := cfg.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var got, want map[string]any
	json.Unmarshal(encoded, &got)
	json.Unmarshal([]byte(raw), &want)
	for _, key := range []string{"dns", "burstObservatory"} {
		if !reflect.DeepEqual(got[key], want[key]) {
			t.Errorf("lost %s", key)
		}
	}
	if !reflect.DeepEqual(get(t, got, "routing.balancers"), get(t, want, "routing.balancers")) {
		t.Fatal("lost balancers")
	}
	tags := get(t, got, "routing.rules.0.inboundTag").([]any)
	if !reflect.DeepEqual(tags, []any{"tun-in", "socks-in"}) {
		t.Fatalf("inbound rules disconnected: %v", tags)
	}
	if get(t, got, "outbounds.0.streamSettings.xhttpSettings.path") != "/test" {
		t.Fatal("lost transport")
	}
	if get(t, got, "outbounds.0.streamSettings.sockopt.mark") != float64(123) {
		t.Fatal("missing uplink mark")
	}
	if get(t, got, "outbounds.0.streamSettings.sockopt.tcpFastOpen") != true {
		t.Fatal("lost sockopt")
	}
	if got["api"] != nil {
		t.Fatal("provider API listener retained")
	}
	if get(t, got, "log.access") != nil {
		t.Fatal("provider log file retained")
	}
	if get(t, got, "inbounds.1.listen") != "127.0.0.1" {
		t.Fatal("proxy exposed")
	}
}
