package cli

import (
	"fmt"
	"github.com/Tr0sT/happ-cli/internal/check"
	"github.com/Tr0sT/happ-cli/internal/config"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"time"
)

func newCheckCmd() *cobra.Command {
	var port int
	var target string
	var tun bool
	cmd := &cobra.Command{Use: "check", Short: "Probe HTTP(S) through SOCKS; optionally also through the TUN interface", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := storeDir()
			if err != nil {
				return err
			}
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("socks") {
				port = cfg.Connect.SocksPort
			}
			if !cmd.Flags().Changed("url") {
				target = cfg.Connect.CheckURL
			}
			result, err := check.Fetch(cmd.Context(), fmt.Sprintf("127.0.0.1:%d", port), target, 10*time.Second)
			if err != nil {
				return err
			}
			fmt.Printf("SOCKS: OK %s\n", result)
			if tun {
				result, err = check.FetchTunnel(cmd.Context(), "thapp", target, 10*time.Second)
				if err != nil {
					return err
				}
				fmt.Printf("TUN: OK %s\n", result)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "socks", defaultSocksPort, "local SOCKS port")
	cmd.Flags().StringVar(&target, "url", config.Default().Connect.CheckURL, "HTTP(S) URL to probe")
	cmd.Flags().BoolVar(&tun, "tun", false, "also verify traffic through thapp (Linux)")
	return cmd
}

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "service", Short: "Control the happ-cli systemd service (uses existing OS permissions)"}
	for _, action := range []string{"start", "stop", "restart", "status", "logs"} {
		cmd.AddCommand(&cobra.Command{Use: action, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			name := "systemctl"
			argv := []string{action, "happ-cli.service"}
			if action == "status" {
				argv = append(argv, "--no-pager")
			}
			if action == "logs" {
				name = "journalctl"
				argv = []string{"-u", "happ-cli.service", "-n", "80", "--no-pager"}
			}
			child := exec.CommandContext(cmd.Context(), name, argv...)
			child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
			return child.Run()
		}})
	}
	return cmd
}
