{ config, pkgs, lib, ... }:
let
  cfg = config.virtualisation.containerd;

  k3s-cni-plugins = pkgs.buildEnv {
    name = "k3s-cni-plugins";
    paths = with pkgs; [
      cni-plugins
      cni-plugin-flannel
    ];
  };

in {
  imports = [
    ../common/containerd.nix
    ./k3s.nix
  ];

  config = lib.mkIf cfg.enable (lib.mkMerge [
    {
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
    }
    (lib.mkIf cfg.k3sIntegration {
      services.k3s.moreFlags = [
        "--container-runtime-endpoint unix:///run/containerd/containerd.sock"
      ];

      virtualisation.containerd = {
        setNamespace = lib.mkDefault "k8s.io";

        settings.plugins."io.containerd.grpc.v1.cri" = {
          stream_server_address = "127.0.0.1";
          stream_server_port = "10010";
          enable_selinux = false;
          enable_unprivileged_ports = true;
          enable_unprivileged_icmp = true;
          disable_apparmor = true;
          disable_cgroup = true;
          restrict_oom_score_adj = true;
          sandbox_image = "rancher/mirrored-pause:3.6";

          cni = {
            conf_dir = "/var/lib/rancher/k3s/agent/etc/cni/net.d/";
            bin_dir = "${k3s-cni-plugins}/bin";
          };

          containerd.runtimes.runc = {
            runtime_type = "io.containerd.runc.v2";
            options.SystemdCgroup = false;
          };
        };
      };
    })
    (lib.mkIf cfg.nixSnapshotterIntegration {
      virtualisation.containerd = {
        setSnapshotter = lib.mkDefault "nix";
        settings = cfg.lib.mkNixSnapshotterSettings;
      };
    })
    (lib.mkIf (cfg.k3sIntegration && cfg.nixSnapshotterIntegration) {
      services.k3s.moreFlags = [
        "--image-service-endpoint unix:///run/nix-snapshotter/nix-snapshotter.sock"
      ];
    })
  ]);
}
