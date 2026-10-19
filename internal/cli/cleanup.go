package cli

import (
	"fmt"
	"github.com/Tr0sT/happ-cli/internal/tunnel"
	"github.com/spf13/cobra"
)

func newCleanupTunCmd() *cobra.Command {
	return &cobra.Command{Use: "cleanup-tun", Short: "Remove this fork's stale routes and firewall rules (Linux, root)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := tunnel.Cleanup(); err != nil {
				return err
			}
			fmt.Println("happ-cli routing and forwarding rules removed.")
			return nil
		},
	}
}
