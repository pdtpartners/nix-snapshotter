{ config, lib, pkgs, ... }:
let
  cfg = config.services.k3s;

in {
  imports = [
    ../common/k3s.nix
  ];

  config = lib.mkIf cfg.enable {
    environment.extraInit = 
      (lib.optionalString cfg.setEmbeddedContainerd ''
        if [ -z "$CONTAINERD_ADDRESS" ]; then
          export CONTAINERD_ADDRESS="/run/k3s/containerd/containerd.sock"
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
          export KUBECONFIG="/etc/rancher/k3s/k3s.yaml"
        fi
      '');

    services.k3s = {
      extraFlags = [ "--snapshotter ${cfg.snapshotter}" ];
    };

    systemd.services.k3s.path = lib.mkIf (cfg.snapshotter == "nix") [
      pkgs.nix
    ];
  };
}
