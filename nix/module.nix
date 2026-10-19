{ config, lib, pkgs, ... }:
let
  cfg = config.services.happ-cli;
  happ = "${cfg.package}/bin/happ --home /var/lib/happ-cli";
in {
  options.services.happ-cli = {
    enable = lib.mkEnableOption "headless happ-cli VPN";
    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ./package.nix { };
    };
    mode = lib.mkOption {
      type = lib.types.enum [ "proxy" "tun" "tun-direct" ];
      default = "tun";
    };
    checkURL = lib.mkOption {
      type = lib.types.str;
      default = "https://api.github.com";
      description = "HTTP(S) endpoint that must succeed through the proxy and TUN.";
    };
    failover = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Try other cached profiles after repeated health check failures.";
    };
    autoUpdate = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Refresh subscriptions every six hours; restart an active connection after a successful update.";
    };
  };
  config = lib.mkIf cfg.enable {
    environment.systemPackages = [ cfg.package ];
    systemd.services.happ-cli = {
      description = "Headless Happ VPN";
      wantedBy = [ "multi-user.target" ];
      wants = [ "network-online.target" ];
      after = [ "network-online.target" ];
      conflicts = [ "happ-vpn.service" ];
      unitConfig.ConditionPathExists = "/var/lib/happ-cli/state.json";
      serviceConfig = {
        Type = "simple";
        StateDirectory = "happ-cli";
        StateDirectoryMode = "0700";
        UMask = "0077";
        ExecStart = "${happ} connect --mode ${cfg.mode} --health-check --check-url ${lib.escapeShellArg cfg.checkURL} --failover=${lib.boolToString cfg.failover}";
        Restart = "on-failure";
        RestartSec = "15s";
        TimeoutStopSec = "30s";
      } // lib.optionalAttrs (cfg.mode != "proxy") {
        ExecStopPost = "${happ} cleanup-tun";
      };
    };
    systemd.services.happ-cli-update = lib.mkIf cfg.autoUpdate {
      description = "Refresh Happ subscriptions";
      wants = [ "network-online.target" ];
      after = [ "network-online.target" ];
      unitConfig.ConditionPathExists = "/var/lib/happ-cli/state.json";
      serviceConfig = {
        Type = "oneshot";
        StateDirectory = "happ-cli";
        StateDirectoryMode = "0700";
        UMask = "0077";
        ExecStart = "${happ} sub update --all";
        ExecStartPost = "${pkgs.systemd}/bin/systemctl try-restart happ-cli.service";
      };
    };
    systemd.timers.happ-cli-update = lib.mkIf cfg.autoUpdate {
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnBootSec = "5min";
        OnUnitActiveSec = "6h";
        RandomizedDelaySec = "5min";
      };
    };
  };
}
