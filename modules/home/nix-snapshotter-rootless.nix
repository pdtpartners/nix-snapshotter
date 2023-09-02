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
      home.sessionVariablesExtra =
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
        (ns-lib.mkRootlessNixSnapshotterService cfg)
        { Service.Environment = "PATH=${lib.makeBinPath cfg.path}"; };
    }
    (lib.mkIf (cfg.preloadContainerdImages != []) {
      systemd.user.services.preload-containerd-images = lib.recursiveUpdate
        (ns-lib.mkRootlessPreloadContainerdImageService cfg)
        { Service.Environment = "CONTAINERD_ADDRESS=%t/containerd/containerd.sock"; };
    })
  ]);
}
