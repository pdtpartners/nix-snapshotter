{ config, lib, ... }:
let
  cfg = config.services.preload-containerd.rootless;

  preload-lib = config.services.preload-containerd.lib;

in {
  imports = [
    ../common/preload-containerd-rootless.nix
  ];

  config = lib.mkIf cfg.enable {
    systemd.user.services.preload-containerd =
      lib.mkIf
        (cfg.targets != [])
        (preload-lib.mkRootlessPreloadContainerdService cfg);
  };
}
