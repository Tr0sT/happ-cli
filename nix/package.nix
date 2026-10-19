{ lib, buildGo126Module, makeWrapper, iproute2, iptables, systemd
, symlinkJoin, v2ray-geoip, v2ray-domain-list-community }:
let
  assets = symlinkJoin {
    name = "happ-xray-assets";
    paths = [ v2ray-geoip v2ray-domain-list-community ];
  };
in buildGo126Module {
  pname = "happ-cli";
  version = "0.2.0-dev";
  src = lib.cleanSource ../.;
  vendorHash = "sha256-wLAomN66+cSqQ++L70SV5l3HWlkG2BMQd0iSzqbysak=";
  subPackages = [ "cmd/happ" ];
  env.CGO_ENABLED = "0";
  env.GOMAXPROCS = "4";
  nativeBuildInputs = [ makeWrapper ];
  ldflags = [ "-s" "-w" "-X main.version=0.2.0-dev" ];
  postInstall = ''
    wrapProgram $out/bin/happ \
      --prefix PATH : ${lib.makeBinPath [ iproute2 iptables systemd ]} \
      --set-default XRAY_LOCATION_ASSET ${assets}/share/v2ray
  '';
  meta = {
    description = "Headless proxy/VPN client with persistent profiles and health checks";
    homepage = "https://github.com/Tr0sT/happ-cli";
    mainProgram = "happ";
    platforms = lib.platforms.linux;
  };
}
