{ config, lib, ... }:
let
  cfg = config.virtualisation.containerd.rootless;

  ns-lib = config.services.nix-snapshotter.lib;

  proxyEnv = config.networking.proxy.envVars;

in {
  imports = [
    ../common/containerd-rootless.nix
  ];

  config = lib.mkIf cfg.enable {
    environment.extraInit = ''
      if [ -z "$CONTAINERD_ADDRESS" ]; then
        export CONTAINERD_ADDRESS="${cfg.setAddress}"
      fi
    '' +
    (lib.optionalString (cfg.setNamespace != "default") ''
      if [ -z "$CONTAINERD_NAMESPACE" ]; then
        export CONTAINERD_NAMESPACE="${cfg.setNamespace}"
      fi
    '') +
    (lib.optionalString (cfg.setSnapshotter != "") ''
      if [ -z "$CONTAINERD_SNAPSHOTTER" ]; then
        export CONTAINERD_SNAPSHOTTER="${cfg.setSnapshotter}"
      fi
    '');

    systemd.user.services.containerd = lib.mkMerge [
      (ns-lib.convertServiceToNixOS (cfg.lib.mkRootlessContainerdService cfg))
      { environment = proxyEnv; }
    ];
  };
}
