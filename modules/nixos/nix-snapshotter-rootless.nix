{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
    mkIf
    mkOption
    mkPackageOptionMD
    types
  ;

  inherit (config.virtualisation.containerd.rootless)
    nsenter
  ;

  cfg = config.services.nix-snapshotter.rootless;

  settingsFormat = pkgs.formats.toml {};

  configFile = settingsFormat.generate "config.toml" cfg.settings;

in {
  imports = [ ./containerd-rootless.nix ];

  options.services.nix-snapshotter.rootless = {
    enable = mkOption {
      type = types.bool;
      default = false;
      description = lib.mdDoc ''
        This option enables nix-snapshotter and containerd in rootless mode.
        To interact with the containerd daemon, one needs to set
        {command}`CONTAINERD_ADDRESS=$XDG_RUNTIME_DIR/containerd/containerd.sock`.
      '';
    };

    package = mkPackageOptionMD pkgs "nix-snapshotter" { };

    configFile = lib.mkOption {
      type = types.nullOr types.path;
      default = null;
      description = lib.mdDoc ''
       Path to nix-snapshotter config file.
       Setting this option will override any configuration applied by the
       settings option.
      '';
    };

    settings = lib.mkOption {
      type = settingsFormat.type;
      default = {};
      description = lib.mdDoc ''
        Verbatim lines to add to config.toml
      '';
    };
  };

  config = mkIf cfg.enable {
    virtualisation.containerd.rootless = {
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

      bindMounts = {
        "$XDG_RUNTIME_DIR/nix-snapshotter".mountPoint = "/run/nix-snapshotter";
        "$XDG_DATA_HOME/nix-snapshotter".mountPoint = "/var/lib/nix-snapshotter";
      };
    };

    systemd.user.services.nix-snapshotter = {
      wantedBy = [ "default.target" ];
      partOf = [ "containerd.service" ];
      after = [ "containerd.service" ];
      description = "nix-snapshotter - containerd snapshotter that understands nix store paths natively (Rootless)";
      serviceConfig = {
        Type = "notify";
        Delegate = "yes";
        KillMode = "mixed";
        Restart = "always";
        RestartSec = "2";
        ExecStart = "${nsenter}/bin/containerd-nsenter ${cfg.package}/bin/nix-snapshotter --log-level debug --config ${configFile}";

        StateDirectory = "nix-snapshotter";
        RuntimeDirectory = "nix-snapshotter";
        RuntimeDirectoryPreserve = "yes";
      };
    };
  };
}
