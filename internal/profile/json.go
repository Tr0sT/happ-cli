package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Tr0sT/happ-cli/internal/link"
)

// Keep each complete profile: flattening outbounds destroys AUTO routing.
func parseJSONBody(body []byte) ([]*link.Server, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || (body[0] != '[' && body[0] != '{') {
		return nil, nil
	}
	profiles := []json.RawMessage{body}
	if body[0] == '[' {
		if err := json.Unmarshal(body, &profiles); err != nil {
			return nil, fmt.Errorf("invalid subscription JSON: %w", err)
		}
	}
	servers := make([]*link.Server, 0, len(profiles))
	seen := map[string]bool{}
	for i, raw := range profiles {
		s, err := link.Parse(string(raw))
		if err != nil {
			return nil, fmt.Errorf("profile %d: %w", i+1, err)
		}
		if seen[s.Key()] {
			return nil, fmt.Errorf("duplicate profile name %q", s.Tag)
		}
		seen[s.Key()] = true
		servers = append(servers, s)
	}
	return servers, nil
}
