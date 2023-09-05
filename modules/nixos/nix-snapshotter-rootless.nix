{ config, lib, ... }:
let
  cfg = config.services.nix-snapshotter.rootless;

  ns-lib = config.services.nix-snapshotter.lib;

in {
  imports = [
    ../common/nix-snapshotter-rootless.nix
    ./containerd-rootless.nix
  ];

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

      systemd.user.services.nix-snapshotter = lib.recursiveUpdate
        (ns-lib.convertServiceToNixOS (ns-lib.mkRootlessNixSnapshotterService cfg))
        { inherit (cfg) path; };
    }
    {
      systemd.user.services.preload-containerd-images = lib.recursiveUpdate
        (ns-lib.convertServiceToNixOS (ns-lib.mkRootlessPreloadContainerdImageService cfg))
        { environment.CONTAINERD_ADDRESS = "%t/containerd/containerd.sock"; };
    }
  ]);
}
