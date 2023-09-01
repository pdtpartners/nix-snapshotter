{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkOption
    types
  ;

  cfg = config.services.nix-snapshotter.rootless;

  ns-lib = config.services.nix-snapshotter.lib;

in {
  imports = [ ./nix-snapshotter-lib.nix ];

  options.services.nix-snapshotter.rootless = {
    inherit (ns-lib.options)
      configFile
      package
      path
      preloadContainerdImages
      setContainerdNamespace
      setContainerdSnapshotter
      settings
    ;

    enable = mkOption {
      type = types.bool;
      default = false;
      description = lib.mdDoc ''
        This option enables nix-snapshotter and containerd in rootless mode.
        To interact with the containerd daemon, one needs to set
        {command}`CONTAINERD_ADDRESS=$XDG_RUNTIME_DIR/containerd/containerd.sock`.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    services.nix-snapshotter.rootless = {
      configFile =
        lib.mkOptionDefault
          (ns-lib.settingsFormat.generate "config.toml" cfg.settings);
    };

    virtualisation.containerd.rootless = {
      enable = true;

      # Configure containerd with nix-snapshotter.
      settings = ns-lib.mkContainerdSettings;

      bindMounts = {
        "$XDG_RUNTIME_DIR/nix-snapshotter".mountPoint = "/run/nix-snapshotter";
        "$XDG_DATA_HOME/nix-snapshotter".mountPoint = "/var/lib/containerd/io.containerd.snapshotter.v1.nix";
      };
    };
  };
}
