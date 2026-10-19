package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Tr0sT/happ-cli/internal/link"
	"github.com/Tr0sT/happ-cli/internal/store"
	"github.com/spf13/cobra"
)

// selectServer picks a server from the list by selector:
//   - "" selects the first server;
//   - a number selects by 1-based index;
//   - anything else is a case-insensitive substring match on the tag.
//
// It returns the server and its 0-based index.
func selectServer(servers []*link.Server, selector string) (*link.Server, int, error) {
	if len(servers) == 0 {
		return nil, 0, fmt.Errorf("no servers available (add a subscription first)")
	}

	selector = strings.TrimSpace(selector)
	if selector == "" {
		return servers[0], 0, nil
	}

	if n, err := strconv.Atoi(selector); err == nil {
		if n < 1 || n > len(servers) {
			return nil, 0, fmt.Errorf("server index %d out of range (1..%d)", n, len(servers))
		}
		return servers[n-1], n - 1, nil
	}

	for _, exact := range []bool{true, false} {
		match := -1
		for i, s := range servers {
			matches := strings.EqualFold(s.Tag, selector) || s.Key() == selector
			if !exact {
				matches = strings.Contains(strings.ToLower(s.Tag), strings.ToLower(selector))
			}
			if !matches {
				continue
			}
			if match >= 0 {
				return nil, 0, fmt.Errorf("ambiguous selector %q; use a server number", selector)
			}
			match = i
		}
		if match >= 0 {
			return servers[match], match, nil
		}
	}
	return nil, 0, fmt.Errorf("no server matches %q", selector)
}

func selectedServer(sub store.SubEntry, selector string) (*link.Server, int, error) {
	if selector != "" || sub.Selected == "" {
		return selectServer(sub.Servers(), selector)
	}
	for i, s := range sub.Servers() {
		if s.Key() == sub.Selected {
			return s, i, nil
		}
	}
	return nil, 0, fmt.Errorf("selected server disappeared from %q; choose explicitly with 'happ select'", sub.Name)
}

func newSelectCmd(deps *Deps) *cobra.Command {
	var subName string
	var working bool
	var target string
	cmd := &cobra.Command{
		Use: "select [selector]", Short: "Persist a server, or find one with --working",
		Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeServerSelector(deps),
		RunE: func(cmd *cobra.Command, args []string) error {
			sub, err := resolveSub(deps.Store, subName)
			if err != nil {
				return err
			}
			if working {
				if len(args) != 0 {
					return fmt.Errorf("use either a selector or --working")
				}
				return selectWorking(cmd.Context(), deps.Store, sub, target)
			}
			if len(args) != 1 {
				return fmt.Errorf("provide a selector or --working")
			}
			s, _, err := selectServer(sub.Servers(), args[0])
			if err != nil {
				return err
			}
			if err := deps.Store.Select(sub.Name, s.Key()); err != nil {
				return err
			}
			fmt.Printf("Selected %s in %s. Restart the connection to apply.\n", s.Tag, sub.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&subName, "sub", "", "subscription name (default: active)")
	cmd.Flags().BoolVar(&working, "working", false, "probe profiles through a temporary SOCKS proxy and save the first working one")
	cmd.Flags().StringVar(&target, "url", "https://api.github.com", "HTTP(S) URL that the selected profile must reach")
	return cmd
}
