{ config, lib, ... }:
let
  cfg = config.services.preload-containerd;

  ns-lib = config.services.nix-snapshotter.lib;

in {
  imports = [
    ../common/preload-containerd.nix
  ];

  config = lib.mkIf cfg.enable {
    systemd.services.preload-containerd =
      lib.mkIf
        (cfg.targets != [])
        (lib.mkMerge [
          (ns-lib.convertServiceToNixOS (cfg.lib.mkPreloadContainerdService cfg))
          {
            description = "Preload images to containerd";
            wantedBy = [ "multi-user.target" ];
          }
        ]);
  };
}
