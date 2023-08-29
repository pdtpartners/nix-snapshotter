{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
    mkIf
    mkOption
    mkPackageOptionMD
    types
  ;

  cfg = config.services.nix-snapshotter;

  settingsFormat = pkgs.formats.toml {};

  configFile = settingsFormat.generate "config.toml" cfg.settings;

in {
  options.services.nix-snapshotter = {

    setContainerdSnapshotter = mkOption {
      type = types.bool;
      default = false;
      description = lib.mdDoc ''
        "Set the nix snapshotter to be the default containerd snapshotter 
        by setting the env var CONTAINERD_SNAPSHOTTER="nix".
      '';
    };

    enable = mkEnableOption "nix-snapshotter";

    package = mkPackageOptionMD pkgs "nix-snapshotter" { };

    configFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = lib.mdDoc ''
       Path to nix-snapshotter config file.
       Setting this option will override any configuration applied by the
       settings option.
      '';
    };

    settings = mkOption {
      type = settingsFormat.type;
      default = {};
      description = lib.mdDoc ''
        Verbatim lines to add to config.toml
      '';
    };
  };

  config = mkIf cfg.enable {

    environment.sessionVariables = lib.mkIf (cfg.setContainerdSnapshotter) {
      CONTAINERD_SNAPSHOTTER  = "nix";
    };

    virtualisation.containerd = {
      enable = true;

      # Configure containerd with nix-snapshotter.
      settings = {
        plugins."io.containerd.grpc.v1.cri" = {
          containerd.snapshotter = "nix";
        };

        proxy_plugins.nix = {
          type = "snapshot";
          address = "/run/nix-snapshotter/nix-snapshotter.sock";
        };
      };
    };

    systemd.services.nix-snapshotter = {
      wantedBy = [ "multi-user.target" ]; 
      after = [ "network.target" ];
      partOf = [ "containerd.service" ];
      description = "nix-snapshotter - containerd snapshotter that understands nix store paths natively";
      serviceConfig = {
        Type = "notify";
        Delegate = "yes";
        KillMode = "mixed";
        Restart = "always";
        RestartSec = "2";
        ExecStart = "${cfg.package}/bin/nix-snapshotter --config ${configFile}";

        StateDirectory = "nix-snapshotter";
        RuntimeDirectory = "nix-snapshotter";
        RuntimeDirectoryPreserve = "yes";
      };
    };
  };
}
