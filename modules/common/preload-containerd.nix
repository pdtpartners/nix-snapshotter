{ pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
    mkOption
    types
  ;

  options = {
    enable = mkEnableOption "preload-containerd";

    targets = mkOption {
      type = types.listOf targetType;
      default = [];
      description = lib.mdDoc ''
        Specify a list of containerd targets to preload image tar archives.
        Each target can specify a different address and namespace.
      '';
    };
  };

  targetType = types.submodule {
    options = {
      archives = mkOption {
        type = types.listOf types.package;
        default = [];
        description = lib.mdDoc ''
          Specify image tar archives to be preloaded to this containerd target.
        '';
      };

      address = mkOption {
        type = types.str;
        default = "/run/containerd/containerd.sock";
        description = lib.mdDoc ''
          Set the containerd address for preloading.
        '';
      };

      namespace = mkOption {
        type = types.str;
        default = "default";
        description = lib.mdDoc ''
          Set the containerd namespace for preloading.
        '';
      };
    };
  };

  mkPreloadContainerdService = cfg:
    let
      preload = pkgs.writeShellApplication {
        name = "preload";
        runtimeInputs = [ pkgs.nix-snapshotter ];
        text = lib.concatStringsSep "\n"
          (lib.concatMap
            (target:
              builtins.map
                (archive: ''
                  nix2container \
                    -a "${target.address}" \
                    -n "${target.namespace}" \
                    load ${archive}
                '')
                target.archives
            )
            cfg.targets
          );
      };

    in {
      Unit = {
        Description = "Preload images to containerd";
        Wants = [ "containerd.service" "nix-snapshotter.service" ];
        After = [ "containerd.service" "nix-snapshotter.service" ];
      };

      Service = {
        Type = "oneshot";
        ExecStart = "${preload}/bin/preload";
        Restart = "on-failure";
        RestartSec = "1s";
        RemainAfterExit = true;
      };
    };

  mkRootlessPreloadContainerdService = cfg: lib.recursiveUpdate
    (mkPreloadContainerdService cfg)
    {
      Unit = {
        Description = "Preload images to containerd (Rootless)";
      };

      Install = {
        WantedBy = [ "default.target" ];
      };
    };

in {
  options.services.preload-containerd = {
    inherit (options)
      enable
      targets
    ;

    lib = mkOption {
      type = types.attrs;
      description = lib.mdDoc "Common functions for preload-containerd.";
      default = {
        inherit
          options
          mkPreloadContainerdService
          mkRootlessPreloadContainerdService
        ;
      };
      internal = true;
    };
  };
}
