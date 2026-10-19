# happ-cli

A headless Linux VPN/proxy client based on [GenkaOk/happ-cli](https://github.com/GenkaOk/happ-cli), originally by [aimuzov](https://github.com/aimuzov). It runs an embedded Xray core, without the Happ desktop application or happd.

[Русская документация](README.ru.md)

The main workflow is simple: fetch a subscription, find a profile that can actually reach the required site, save that choice, and reconnect on persistent failures. An AUTO profile is optional.

## Build

```sh
nix build github:Tr0sT/happ-cli
./result/bin/happ --help
```

Or build with Go 1.26.4 or newer:

```sh
CGO_ENABLED=0 go build -o happ ./cmd/happ
```

Build from the checkout: the upstream gVisor compatibility replacement in `go.mod` must be honored. The Nix package includes iproute2, iptables and Xray geoip/geosite assets. Standalone binaries need those tools for Linux TUN and `XRAY_LOCATION_ASSET` pointing to a directory containing `geoip.dat` / `geosite.dat` if a profile references them.

## Commands

```sh
# Keep the subscription URL out of shell history and process arguments.
happ sub add --name primary --url-file /path/to/private-subscription-url
happ sub list
happ sub use primary
happ list

# Probe profiles through temporary local proxies, without changing routes.
happ select --working --url https://api.github.com
happ select 'Germany'                 # or explicitly choose a name / number
happ connect --health-check --failover
happ check --url https://api.github.com

happ sub update --all                 # noninteractive; keeps cached data on failure
happ config-show
happ config-init                     # deliberately overwrites config.yaml with defaults
```

`--home DIR` works with all commands; the default is `~/.config/happ-cli`. `--url-file -` reads stdin. `happ://add/https://...` subscription links are accepted. `--ua` overrides the subscription User-Agent.

`select --working` chooses the **first passing profile**, not the one with the lowest ping. It verifies HTTP 2xx through that profile's SOCKS proxy, with an eight-second request deadline per profile. It does not change routes or restart a running service. If all probes fail, the previous selection is kept. Use an endpoint that matters to your workload.

Selection survives reordered subscriptions and changed credentials. JSON profiles use their unique `remarks` name as identity; share links use protocol/name/address/port. If a selected profile disappears, a normal connection fails explicitly; `--failover` allows other profiles. Failover tries the cached profiles once per process, leaves the preferred choice intact, and reports failure if none work. systemd then retries after 15 seconds.

`sub update` validates generated Xray configs before replacing cached data. Empty responses and losses exceeding half the profile count are errors; `--accept-shrink` explicitly permits the latter. Configuration validation does not prove that a provider is reachable. The update timer and command can run alongside selection without erasing it.

## Linux service and TUN

The flake exports `nixosModules.default`:

```nix
# flake inputs
happ-cli.url = "github:Tr0sT/happ-cli";
happ-cli.inputs.nixpkgs.follows = "nixpkgs";

# NixOS module imports and settings
imports = [ inputs.happ-cli.nixosModules.default ];
services.happ-cli.enable = true;
services.happ-cli.mode = "tun";
services.happ-cli.checkURL = "https://api.github.com";
services.happ-cli.failover = true;
services.happ-cli.autoUpdate = true;
```

The service uses `/var/lib/happ-cli` (0700, root-owned). Populate that directory with `happ --home /var/lib/happ-cli sub add ...` and `select --working`, using your normal privilege mechanism. Only one VPN should own the default route: the module conflicts with `happ-vpn.service`. Apply NixOS changes manually. This repository never runs a system rebuild itself.

Service commands delegate to systemd and use its existing permissions:

```sh
happ service start
happ service stop
happ service restart
happ service status
happ service logs
happ --home /var/lib/happ-cli check --tun --url https://api.github.com
```

The six-hour update timer restarts an already-running service only after successful refresh. It does not turn a manually stopped service back on. A change to the saved selection takes effect on restart.

Both `tun` (tun2socks) and `tun-direct` (native Xray TUN) support periodic SOCKS **and interface-bound TUN** probes. Three consecutive failures cause a nonzero exit or failover. SIGINT/SIGTERM cleanly remove installed routes. Proxy mode needs no root; TUN requires root. Local proxies listen on loopback only.

Routing uses table 20260, priorities 20257–20260, protocol 242 and socket mark `0x48415050`. It consults the existing main table for connected/static routes (including Docker and WARP veth networks), preserves private LAN routes and TCP source-port 22 replies, and excludes marked Xray uplink/bootstrap DNS sockets from the tunnel. Ordinary global IPv6 is rejected rather than silently bypassing the IPv4 tunnel. `cleanup-tun` removes this fork's tagged leftovers and refuses to operate while its TUN process is running. It never flushes upstream's old table 100.

This is **fail-open**: after stopping/crashing, normal networking can resume. It is not a kill switch. LAN DNS and bootstrap resolution can use the ordinary uplink. SSH on a custom port across a public route needs an explicit bypass route before enabling the VPN. Additional VPNs or earlier policy rules can override the intended route and require host-specific inspection.

## Complete Xray profiles

JSON objects, arrays and base64-wrapped JSON are stored intact, retaining outbounds, DNS, transports, routing, balancers and observatories. The CLI supplies its own loopback/TUN listeners, maps corresponding inbound routing tags, removes provider API/metrics listeners and file-log settings, and adds Linux socket marks for TUN. Only import subscriptions you trust. Provider-specific unsupported Xray features produce a validation error; a JSON parser cannot make them compatible.

Share links support VLESS, VMess, Trojan and Shadowsocks. Hysteria2 share links can be listed but are not converted by this client. A raw profile is supported only if the embedded Xray version accepts it. This fork prioritizes Linux/NixOS; macOS proxy builds remain available, while complete JSON TUN and native TUN routing are Linux-only.

`state.json` contains subscription URLs and credentials; `device.json` contains persistent HWID/UUID. Keep both private and backed up. A corrupt identity is an error, never a reason to silently register a new device. Copy a known identity before fetching a device-limited subscription. `happ config` prints credentials and should not be pasted into public issue reports.

## Verification

```sh
go test ./...
go vet ./...
bash tests/netns.sh
```

The network test creates isolated user, mount and network namespaces. It checks both TUN implementations with a real encrypted proxy, Docker/WARP-style routes, forwarded traffic, IPv6 rejection and teardown. It does not change host networking. Unprivileged user namespaces, iproute2, iptables and `/dev/net/tun` must be available. A passing isolated test does not replace testing the actual provider and VDS routing.

Useful reference: [d4nom/happ-vpn-setup](https://github.com/d4nom/happ-vpn-setup) documents stable HWID, repeated-failure health checks and validation before refresh. This fork retains Xray profiles rather than translating them to sing-box.
