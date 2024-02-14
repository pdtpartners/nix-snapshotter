{ config, lib, ... }:
let
  cfg = config.services.k3s.rootless;

in {
  imports = [
    ../common/k3s-rootless.nix
  ];

  config = lib.mkIf cfg.enable {
    systemd.user.services.k3s = cfg.lib.mkRootlessK3sService cfg;

    home.sessionVariablesExtra = 
      (lib.optionalString cfg.setEmbeddedContainerd ''
        if [ -z "$CONTAINERD_ADDRESS" ]; then
          export CONTAINERD_ADDRESS="$XDG_RUNTIME_DIR/k3s/containerd/containerd.sock"
        fi
        if [ -z "$CONTAINERD_NAMESPACE" ]; then
          export CONTAINERD_NAMESPACE="k8s.io"
        fi
        if [ -z "$CONTAINERD_SNAPSHOTTER" ]; then
          export CONTAINERD_SNAPSHOTTER="${cfg.snapshotter}"
        fi
      '') +
      (lib.optionalString cfg.setKubeConfig ''
        if [ -z "$KUBECONFIG" ]; then
          export KUBECONFIG="$HOME/.kube/k3s.yaml"
        fi
      '');
  };
}
