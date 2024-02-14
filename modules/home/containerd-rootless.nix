{ config, lib, ... }:
let
  cfg = config.virtualisation.containerd.rootless;

in {
  imports = [
    ../common/containerd-rootless.nix
  ];

  config = lib.mkIf cfg.enable {
    home.sessionVariablesExtra = ''
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

    systemd.user.services.containerd = cfg.lib.mkRootlessContainerdService cfg;
  };
}
