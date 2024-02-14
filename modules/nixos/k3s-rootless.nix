{ config, lib, ... }:
let
  cfg = config.services.k3s.rootless;

  ns-lib = config.services.nix-snapshotter.lib;

in {
  imports = [
    ../common/nix-snapshotter.nix
    ../common/k3s-rootless.nix
  ];

  config = lib.mkIf cfg.enable {
    systemd.user.services.k3s =
      ns-lib.convertServiceToNixOS (cfg.lib.mkRootlessK3sService cfg);

    environment.extraInit = 
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
        if [ -z "$ROOTLESSKIT_STATE_DIR" ]; then
          export ROOTLESSKIT_STATE_DIR="$HOME/.rancher/k3s/rootless"
        fi
      '') +
      (lib.optionalString cfg.setKubeConfig ''
        if [ -z "$KUBECONFIG" ]; then
          export KUBECONFIG="$HOME/.kube/k3s.yaml"
        fi
      '');

    # Crucial to enable k3s rootless mode.
    # See: https://rootlesscontaine.rs/getting-started/common/cgroup2/#enabling-cpu-cpuset-and-io-delegation
    systemd.services."user@".serviceConfig.Delegate = "memory pids cpu cpuset";
    boot.kernel.sysctl."net.ipv4.ip_forward" = "1";
  };
}
