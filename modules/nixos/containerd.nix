{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkOption
    types
  ;

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

  options.virtualisation.containerd = {
    listenOptions = mkOption {
      type = types.listOf types.str;
      default = [ cfg.setAddress ];
      description = ''
        A list of unix and tcp containerd should listen to. The format follows
        ListenStream as described in systemd.socket(5).
      '';
    };
  };

  config = lib.mkIf cfg.enable (lib.mkMerge [
    {
      ids.gids.containerd = 888; # unused gid.

      users.groups.containerd.gid = config.ids.gids.containerd;

      virtualisation.containerd = {
        settings = lib.recursiveUpdate
          (cfg.lib.mkSettings cfg)
          {
            grpc.gid = config.ids.gids.containerd;
          };
      };

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

      systemd.sockets.contianerd = {
        description = "Containerd Socket for the API";
        wantedBy = [ "sockets.target" ];
        socketConfig = {
          ListenStream = cfg.listenOptions;
          SocketMode = "0660";
          SocketUser = "root";
          SocketGroup = "containerd";
        };
      };

      systemd.services.containerd.path = cfg.path;
    }
    (lib.mkIf cfg.k3sIntegration {
      services.k3s.moreFlags = [
        "--container-runtime-endpoint unix:///run/containerd/containerd.sock"
      ];

      virtualisation.containerd = {
        setNamespace = lib.mkDefault "k8s.io";

        settings = {
          plugins."io.containerd.grpc.v1.cri" = {
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
    (lib.mkIf cfg.gVisorIntegration {
      virtualisation.containerd = {
        path = [ pkgs.gvisor ];
        settings = cfg.lib.mkGVisorSettings;
      };
    })
  ]);
}
