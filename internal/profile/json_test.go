package profile

import (
	"encoding/base64"
	"encoding/json"
	"github.com/Tr0sT/happ-cli/internal/link"
	"reflect"
	"testing"
)

const autoProfile = `{"remarks":"AUTO","dns":{"servers":["1.1.1.1"]},"outbounds":[{"tag":"one","protocol":"vless","settings":{"vnext":[{"address":"one.example","port":443,"users":[{"id":"uuid"}]}]},"streamSettings":{"network":"xhttp","xhttpSettings":{"path":"/a"}}},{"tag":"two","protocol":"trojan"},{"tag":"direct","protocol":"freedom"}],"routing":{"balancers":[{"tag":"auto","selector":["one","two"],"strategy":{"type":"leastPing"}}],"rules":[{"type":"field","network":"tcp,udp","balancerTag":"auto"}]},"burstObservatory":{"subjectSelector":["one","two"]}}`

func TestJSONProfilesSurviveStoreRoundTrip(t *testing.T) {
	for _, body := range []string{autoProfile, "[" + autoProfile + "]", base64.StdEncoding.EncodeToString([]byte(autoProfile))} {
		servers, err := ParseBody([]byte(body))
		if err != nil || len(servers) != 1 {
			t.Fatalf("parse: %v, count %d", err, len(servers))
		}
		s, err := link.Parse(servers[0].Raw)
		if err != nil {
			t.Fatal(err)
		}
		var want, got any
		json.Unmarshal([]byte(autoProfile), &want)
		json.Unmarshal(s.RawConfig, &got)
		if s.Protocol != "xray" || s.Tag != "AUTO" || !reflect.DeepEqual(want, got) {
			t.Fatal("complete AUTO profile was changed")
		}
	}
}

func TestJSONRejectsInvalidProfiles(t *testing.T) {
	for _, raw := range []string{"[not json", `{}`, `{"remarks":"direct","outbounds":[{"protocol":"freedom"}]}`, `{"outbounds":[{"protocol":"vless"}]}`, "[" + autoProfile + "," + autoProfile + "]"} {
		if _, err := ParseBody([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestSelectionKeySurvivesCredentialRotation(t *testing.T) {
	a, _ := link.Parse("trojan://old@host:443#Server")
	b, _ := link.Parse("trojan://new@host:443#Server")
	if a.Key() != b.Key() {
		t.Fatal("credential rotation changed identity")
	}
}
