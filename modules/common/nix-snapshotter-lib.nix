{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkOption
    mkPackageOptionMD
    types
  ;

  inherit (pkgs.go)
    GOOS
    GOARCH
  ;

  inherit (config.virtualisation.containerd.rootless)
    nsenter
  ;

  options = {
    configFile = mkOption {
      type = types.nullOr types.path;
      description = lib.mdDoc ''
       Path to nix-snapshotter config file.
       Setting this option will override any configuration applied by the
       settings option.
      '';
    };

    package = mkPackageOptionMD pkgs "nix-snapshotter" { };

    path = mkOption {
      type = types.listOf types.package;
      default = [ pkgs.nix ];
      description = lib.mdDoc ''
        Set the path of the nix-snapshotter service, if it requires access to
        alternative nix binaries.
      '';
    };

    preloadContainerdImages = mkOption {
      type = types.listOf types.package;
      default = [];
      description = lib.mdDoc ''
        Specify image tar archives that should be preloaded into containerd.
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

    settings = mkOption {
      type = settingsFormat.type;
      default = {};
      description = lib.mdDoc ''
        Verbatim lines to add to config.toml
      '';
    };
  };

  settingsFormat = pkgs.formats.toml {};

  mkContainerdSettings = {
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

  # SYSTEMD USER SERVICES
  # ---------------------
  #
  # When writing systemd user services targetting both NixOS modules and
  # home-manager modules, the easiest transformation is home-manager -> NixOS,
  # which is why the services are written as such.

  # Converts a home-manager systemd user service to a NixOS systemd user
  # service. Since home-manager style services map closer to raw systemd
  # service specification, it's easier to transform in this direction.
  convertServiceToNixOS = unit:
    {
      serviceConfig = lib.optionalAttrs (unit?Service) unit.Service;
      unitConfig = lib.optionalAttrs (unit?Unit) unit.Unit;
    } // (lib.optionalAttrs (unit?Install.WantedBy) {
      # Only `WantedBy` is supported by NixOS as [Install] fields are not
      # supported, due to its stateful nature.
      wantedBy = unit.Install.WantedBy;
    });

  mkNixSnapshotterService = {
    Service = {
      Type = "notify";
      Delegate = "yes";
      KillMode = "mixed";
      Restart = "always";
      RestartSec = "2";

      StateDirectory = "nix-snapshotter";
      RuntimeDirectory = "nix-snapshotter";
      RuntimeDirectoryPreserve = "yes";
    };
  };

  mkRootlessNixSnapshotterService = cfg: lib.recursiveUpdate
    mkNixSnapshotterService
    {
      Unit = {
        Description = "nix-snapshotter - containerd snapshotter that understands nix store paths natively (Rootless)";
        After = [ "containerd.service" ];
        PartOf = [ "containerd.service" ];
      };

      Install = {
        WantedBy = [ "default.target" ];
      };

      Service.ExecStart = "${nsenter}/bin/containerd-nsenter ${cfg.package}/bin/nix-snapshotter --config ${cfg.configFile}";
    };

  mkPreloadContainerdImageService = cfg:
    let
      namespace = cfg.setContainerdNamespace;

      archives = cfg.preloadContainerdImages;

      preload = pkgs.writeShellScriptBin "preload" (
        lib.concatStringsSep "\n"
          (builtins.map
            (archive: ''${pkgs.nix-snapshotter}/bin/nix2container -n "${namespace}" load ${archive}'' )
            archives
          )
      );

    in {
      Unit = {
        Wants = [ "containerd.service" "nix-snapshotter.service" ];
        After = [ "containerd.service" "nix-snapshotter.service" ];
      };

      Service = {
        Type = "oneshot";
        ExecStart = "${preload}/bin/preload";
        RemainAfterExit = true;
      };
    };

  mkRootlessPreloadContainerdImageService = cfg:
    lib.recursiveUpdate
      (mkPreloadContainerdImageService cfg)
      {
        Unit = {
          Description = "Preload images to containerd (Rootless)";
        };
        Install = {
          WantedBy = [ "default.target" ];
        };
      };

in {
  options.services.nix-snapshotter = {
    lib = mkOption {
      description = lib.mdDoc "Common functions for the nix-snapshotter modules.";
      default = {
        inherit options;
        inherit settingsFormat;
        inherit convertServiceToNixOS;
        inherit mkContainerdSettings;
        inherit mkNixSnapshotterService;
        inherit mkRootlessNixSnapshotterService;
        inherit mkPreloadContainerdImageService;
        inherit mkRootlessPreloadContainerdImageService;
      };
      type = types.attrs;
      internal = true;
    };
  };
}
