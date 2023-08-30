{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
    mkOption
    mkPackageOptionMD
    types
  ;

  inherit (pkgs.go)
    GOOS
    GOARCH
  ;

  cfg = config.services.nix-snapshotter;

  settingsFormat = pkgs.formats.toml {};

  configFile = settingsFormat.generate "config.toml" cfg.settings;

  baseContainerdSettings = {
    plugins."io.containerd.grpc.v1.cri" = {
      containerd.snapshotter = "nix";
    };

    plugins."io.containerd.transfer.v1.local".unpack_config = [{
      platform = "${GOOS}/${GOARCH}";
      snapshotter = "nix";
    }];

    proxy_plugins.nix = {
      type = "snapshot";
      address = "/run/nix-snapshotter/nix-snapshotter.sock";
    };
  };

  baseServiceConfig = {
    Type = "notify";
    Delegate = "yes";
    KillMode = "mixed";
    Restart = "always";
    RestartSec = "2";

    StateDirectory = "nix-snapshotter";
    RuntimeDirectory = "nix-snapshotter";
    RuntimeDirectoryPreserve = "yes";
  };

  mkPreloadContainerdImageService = { archives, namespace }: {
    wants = [ "containerd.service" "nix-snapshotter.service" ];
    after = [ "containerd.service" "nix-snapshotter.service" ];

    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };

    environment = {
      CONTAINERD_SNAPSHOTTER = "nix";
      CONTAINERD_NAMESPACE = namespace;
    };

    script = lib.concatStringsSep "\n"
      (builtins.map
        (archive: "${pkgs.containerd}/bin/ctr image import --local=false ${archive}" )
        archives
      );
  };

in {
  options.services.nix-snapshotter = {
    enable = mkEnableOption "nix-snapshotter";

    package = mkPackageOptionMD pkgs "nix-snapshotter" { };

    path = mkOption {
      type = types.listOf types.package;
      default = [ pkgs.nix ];
      description = lib.mdDoc ''
        Set the path of the nix-snapshotter service, if it requires access to
        alternative nix binaries.
      '';
    };

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

    setContainerdSnapshotter = mkOption {
      type = types.bool;
      default = false;
      description = lib.mdDoc ''
        Set the nix snapshotter to be the default containerd snapshotter
        by setting the env var CONTAINERD_SNAPSHOTTER="nix".
      '';
    };

    setContainerdNamespace = mkOption {
      type = types.str;
      default = "default";
      description = lib.mdDoc ''
        Set the default containerd namespace by setting the env var
        CONTAINERD_NAMESPACE.
      '';
    };

    preloadContainerdImages = mkOption {
      type = types.listOf types.package;
      default = [];
      description = lib.mdDoc ''
        Specify image tar archives that should be preloaded into containerd.
      '';
    };

    lib = mkOption {
      description = lib.mdDoc "Common functions for the kubernetes modules.";
      default = {
        inherit baseContainerdSettings;
        inherit baseServiceConfig;
        inherit mkPreloadContainerdImageService;
      };
      type = types.attrs;
      internal = true;
    };
  };

  config = lib.mkIf cfg.enable (lib.mkMerge [
    {
      environment.extraInit =
        (lib.optionalString cfg.setContainerdSnapshotter ''
          if [ -z "$CONTAINERD_SNAPSHOTTER" ]; then
            export CONTAINERD_SNAPSHOTTER="nix"
          fi
        '') +
        (lib.optionalString (cfg.setContainerdNamespace != "default") ''
          if [ -z "$CONTAINERD_NAMESPACE" ]; then
            export CONTAINERD_NAMESPACE="${cfg.setContainerdNamespace}"
          fi
        '');

      virtualisation.containerd = {
        enable = true;
        settings = cfg.lib.baseContainerdSettings;
      };

      systemd.services.nix-snapshotter = {
        inherit (cfg) path;
        description = "nix-snapshotter - containerd snapshotter that understands nix store paths natively";
        wantedBy = [ "multi-user.target" ];
        after = [ "network.target" ];
        partOf = [ "containerd.service" ];
        serviceConfig = lib.mkMerge [
          cfg.lib.baseServiceConfig
          {
            ExecStart = "${cfg.package}/bin/nix-snapshotter --config ${configFile}";
          }
        ];
      };
    }
    (lib.mkIf (cfg.preloadContainerdImages != []) {
      systemd.services.preload-containerd-images = lib.mkMerge [
        (cfg.lib.mkPreloadContainerdImageService {
          archives = cfg.preloadContainerdImages;
          namespace = cfg.setContainerdNamespace;
        })
        {
          description = "Preload images to containerd";
          wantedBy = [ "multi-user.target" ];
        }
      ];
    })
  ]);
}
