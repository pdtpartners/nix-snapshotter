{ pkgs, ... }:
let
  k3s-cni-plugins = pkgs.buildEnv {
    name = "k3s-cni-plugins";
    paths = [
      pkgs.cni-plugins
      pkgs.cni-plugin-flannel
    ];
  };

in {
  services.k3s = {
    enable = true;
    extraFlags = toString [
      "--container-runtime-endpoint unix:///run/containerd/containerd.sock"
      "--image-service-endpoint unix:///run/nix-snapshotter/nix-snapshotter.sock"
    ];
  };

  virtualisation.containerd = {
    settings.plugins."io.containerd.grpc.v1.cri".cni = {
      bin_dir = "${k3s-cni-plugins}/bin";
      conf_dir = "/var/lib/rancher/k3s/agent/etc/cni/net.d/";
    };
  };

  environment.sessionVariables = {
    KUBECONFIG = "/etc/rancher/k3s/k3s.yaml";
  };
}
