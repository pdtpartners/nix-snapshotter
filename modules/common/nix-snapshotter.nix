{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkOption
    mkPackageOption
    types
  ;

  inherit (config.virtualisation.containerd.rootless)
    nsenter
  ;

  settingsFormat = pkgs.formats.toml {};

  options = {
    configFile = mkOption {
      type = types.nullOr types.path;
      description = lib.mdDoc ''
       Path to nix-snapshotter config file.
       Setting this option will override any configuration applied by the
       settings option.
      '';
    };

    package = mkPackageOption pkgs "nix-snapshotter" { };

    path = mkOption {
      type = types.listOf types.package;
      default = [ pkgs.nix ];
      description = lib.mdDoc ''
        Set the path of the nix-snapshotter service, if it requires access to
        alternative nix binaries.
      '';
    };

    extraFlags = mkOption {
      type = types.listOf types.str;
      default = [ ];
      description = lib.mkDoc ''
        Additional command line flags to pass to nix-snapshotter
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

  # SYSTEMD USER SERVICES
  # ---------------------
  #
  # When writing systemd user services targetting both NixOS modules and
  # home-manager modules, the easiest transformation is home-manager -> NixOS,
  # which is why the services are written as such.

  # Converts a home-manager systemd user service to a NixOS systemd user
  # service. Since home-manager style services map closer to raw systemd
  # service specification, it's easier to transform in this direction.
  convertServiceToNixOS = unit: lib.mkMerge [
    (lib.mkIf (unit ? Service) {
      serviceConfig = unit.Service;
    })
    (lib.mkIf (unit ? Unit) {
      unitConfig = unit.Unit;
    })
    (lib.mkIf (unit ? Install.WantedBy) {
      # Only `WantedBy` is supported by NixOS as [Install] fields are not
      # supported, due to its stateful nature.
      wantedBy = unit.Install.WantedBy;
    })
  ];

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

      Service.ExecStart = builtins.concatStringsSep " " ([
        "${nsenter}/bin/containerd-nsenter"
        "${cfg.package}/bin/nix-snapshotter"
        "--config ${cfg.configFile}"
      ] ++ cfg.extraFlags);
    };

in {
  options.services.nix-snapshotter = {
    lib = mkOption {
      type = types.attrs;
      description = lib.mdDoc "Common functions for the nix-snapshotter modules.";
      default = {
        inherit
          options
          convertServiceToNixOS
          mkNixSnapshotterService
          mkRootlessNixSnapshotterService
        ;
      };
      internal = true;
    };
  };
}
