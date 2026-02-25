{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkOption
    types
  ;

  cfg = config.services.nix-snapshotter.rootless;

  ns-lib = config.services.nix-snapshotter.lib;

  settingsFormat = pkgs.formats.toml {};

in {
  imports = [
    ./nix-snapshotter.nix
  ];

  options.services.nix-snapshotter.rootless = {
    inherit (ns-lib.options)
      configFile
      package
      path
      extraFlags
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
          (settingsFormat.generate "config.toml" cfg.settings);
    };
  };
}
