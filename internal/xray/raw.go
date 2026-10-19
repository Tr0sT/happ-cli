package xray

import (
	"encoding/json"
	"fmt"
	"github.com/Tr0sT/happ-cli/internal/link"
)

func buildRawConfig(s *link.Server, opts Options) (*Config, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(s.RawConfig, &raw); err != nil {
		return nil, err
	}
	inbounds, err := buildInbounds(opts)
	if err != nil {
		return nil, err
	}
	// Keep inbound-specific routing meaningful after replacing provider listeners.
	var previous []struct {
		Tag      string
		Protocol string
	}
	if data := raw["inbounds"]; data != nil {
		if err := json.Unmarshal(data, &previous); err != nil {
			return nil, err
		}
	}
	if data := raw["routing"]; data != nil {
		var routing map[string]json.RawMessage
		if err := json.Unmarshal(data, &routing); err != nil {
			return nil, err
		}
		if data := routing["rules"]; data != nil {
			var rules []map[string]json.RawMessage
			if err := json.Unmarshal(data, &rules); err != nil {
				return nil, err
			}
			for _, rule := range rules {
				if rule["inboundTag"] == nil {
					continue
				}
				var tags []string
				if err := json.Unmarshal(rule["inboundTag"], &tags); err != nil {
					return nil, err
				}
				var mapped []string
				for _, tag := range tags {
					matched := false
					for _, old := range previous {
						if old.Tag != tag {
							continue
						}
						for _, inbound := range inbounds {
							if old.Protocol == inbound.Protocol || (old.Protocol == "socks" && inbound.Protocol == "tun") {
								mapped = append(mapped, inbound.Tag)
								matched = true
							}
						}
					}
					if !matched {
						mapped = append(mapped, tag)
					}
				}
				rule["inboundTag"], _ = json.Marshal(mapped)
			}
			routing["rules"], _ = json.Marshal(rules)
		}
		raw["routing"], _ = json.Marshal(routing)
	}
	// Provider-supplied listeners and file logs must not open ports or files as root.
	for _, key := range []string{"api", "metrics", "log"} {
		delete(raw, key)
	}
	loglevel := opts.LogLevel
	if loglevel == "" {
		loglevel = "warning"
	}
	raw["log"], _ = json.Marshal(LogConfig{Loglevel: loglevel})
	raw["inbounds"], _ = json.Marshal(inbounds)
	if opts.SocketMark != 0 {
		var outbounds []map[string]json.RawMessage
		if err := json.Unmarshal(raw["outbounds"], &outbounds); err != nil {
			return nil, err
		}
		for _, out := range outbounds {
			stream := map[string]json.RawMessage{}
			if data := out["streamSettings"]; data != nil {
				if err := json.Unmarshal(data, &stream); err != nil {
					return nil, fmt.Errorf("streamSettings: %w", err)
				}
			}
			if stream == nil {
				stream = map[string]json.RawMessage{}
			}
			sock := map[string]json.RawMessage{}
			if data := stream["sockopt"]; data != nil {
				if err := json.Unmarshal(data, &sock); err != nil {
					return nil, fmt.Errorf("sockopt: %w", err)
				}
			}
			if sock == nil {
				sock = map[string]json.RawMessage{}
			}
			sock["mark"], _ = json.Marshal(opts.SocketMark)
			stream["sockopt"], _ = json.Marshal(sock)
			out["streamSettings"], _ = json.Marshal(stream)
		}
		raw["outbounds"], _ = json.Marshal(outbounds)
	}
	return &Config{raw: raw}, nil
}
