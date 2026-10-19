package link

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

func parseConfig(raw string) (*Server, error) {
	var cfg struct {
		Remarks   string `json:"remarks"`
		Outbounds []struct {
			Protocol string `json:"protocol"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("invalid Xray JSON: %w", err)
	}
	proxy := false
	for _, out := range cfg.Outbounds {
		if out.Protocol == "" {
			return nil, fmt.Errorf("Xray outbound has no protocol")
		}
		switch out.Protocol {
		case "freedom", "blackhole", "dns", "loopback":
		default:
			proxy = true
		}
	}
	if !proxy {
		return nil, fmt.Errorf("Xray profile has no proxy outbound")
	}
	if strings.TrimSpace(cfg.Remarks) == "" {
		return nil, fmt.Errorf("Xray profile needs a nonempty remarks name for persistent selection")
	}
	return &Server{Tag: cfg.Remarks, Protocol: "xray", Raw: raw, RawConfig: json.RawMessage(raw)}, nil
}

// Key deliberately excludes credentials, which can rotate on subscription refresh.
func (s *Server) Key() string {
	identity := s.Protocol + "\x00" + s.Tag
	if s.RawConfig == nil {
		identity += fmt.Sprintf("\x00%s\x00%d", s.Address, s.Port)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
}
