{ config, lib, pkgs, ... }:
let
  inherit (lib)
    mkOption
    types
  ;

  cfg = config.services.k3s;

in {
  imports = [
    ../common/k3s.nix
  ];

  options = {
    services.k3s = {
      # This was introduced to provide a mergeable listOf type.
      moreFlags = mkOption {
        type = types.listOf types.str;
        description = lib.mdDoc "Extra flags to pass to the k3s command.";
        default = [];
        example = [ "--no-deploy traefik" "--cluster-cidr 10.24.0.0/16" ];
      };
    };
  };

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
      moreFlags = [ "--snapshotter ${cfg.snapshotter}" ];

      extraFlags = toString cfg.moreFlags;
    };

    systemd.services.k3s.path = lib.mkIf (cfg.snapshotter == "nix") [
      pkgs.nix
    ];
  };
}
