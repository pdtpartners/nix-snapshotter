{ config, lib, ... }:
let
  cfg = config.services.nix-snapshotter.rootless;

  ns-lib = config.services.nix-snapshotter.lib;

in {
  imports = [
    ../common/nix-snapshotter-rootless.nix
  ];

  config = lib.mkIf cfg.enable {
    systemd.user.services.nix-snapshotter = lib.mkMerge [
      (ns-lib.convertServiceToNixOS (ns-lib.mkRootlessNixSnapshotterService cfg))
      { inherit (cfg) path; }
    ];
  };
}
