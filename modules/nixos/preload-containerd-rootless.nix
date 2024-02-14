{ config, lib, ... }:
let
  cfg = config.services.preload-containerd.rootless;

  ns-lib = config.services.nix-snapshotter.lib;

  preload-lib = config.services.preload-containerd.lib;

in {
  imports = [
    ../common/preload-containerd-rootless.nix
  ];

  config = lib.mkIf cfg.enable {
    systemd.user.services.preload-containerd =
      lib.mkIf
        (cfg.targets != [])
        (ns-lib.convertServiceToNixOS (preload-lib.mkRootlessPreloadContainerdService cfg));
  };
}
