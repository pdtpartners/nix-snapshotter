{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
    mkIf
    mkPackageOptionMD
  ;

  cfg = config.nix-snapshotter;

in {
  options.nix-snapshotter = {
    enable = mkEnableOption "nix-snapshotter";

    package = mkPackageOptionMD pkgs "nix-snapshotter" { };
  };

  config = mkIf cfg.enable {
    virtualisation.containerd = {
      enable = true;

      settings = {
        plugins."io.containerd.grpc.v1.cri" = {
          containerd.snapshotter = "nix";
        };

        proxy_plugins.nix = {
          type = "snapshot";
          address = "/run/containerd-nix/containerd-nix.sock";
        };
      };
    };

    systemd.services.nix-snapshotter = {
      wantedBy = [ "multi-user.target" ]; 
      after = [ "network.target" ];
      before = [ "containerd.service" ];
      description = "containerd remote snapshotter for native nix images";
      serviceConfig = {
        Type = "notify";
        Delegate = "yes";
        KillMode = "process";
        Restart = "always";
        RestartSec = "5";
        ExecStart = "${cfg.package}/bin/nix-snapshotter";
      };
    };
  };
}
