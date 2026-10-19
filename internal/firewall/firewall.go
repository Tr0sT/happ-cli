// Package firewall manages the forwarding exceptions owned by happ-cli.
package firewall

func Rules(iface string) (func() error, error) { return rules(iface) }
