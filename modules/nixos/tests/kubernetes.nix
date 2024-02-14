/*
  kubernetes configures Kubernetes with containerd & nix-snapshotter.

*/
{ lib, pkgs, ... }:
{
  nodes.machine = { config, k8sResources, ... }:
    let
      # Only k3s has a builtin `LoadBalancer` so the redisService needs to be
      # updated to use `NodePort`.
      redisService =
        lib.recursiveUpdate
          k8sResources.redisService
          {
            spec = {
              type = "NodePort";
              ports = [{
                name = "client";
                port = 6379;
                nodePort = 30000;
              }];
            };
          };

    in {
      imports = [
        ../containerd.nix
        ../nix-snapshotter.nix
      ];

      virtualisation.containerd = {
        enable = true;
        nixSnapshotterIntegration = true;
      };

      services.nix-snapshotter.enable = true;

      services.kubernetes = {
        roles = ["master" "node"];
        masterAddress = "localhost";
        kubelet.extraOpts = "--image-service-endpoint unix:///run/nix-snapshotter/nix-snapshotter.sock";
      };
  
      environment.systemPackages = with pkgs; [
        redis
        kubectl
      ];

      environment.sessionVariables = {
        KUBECONFIG = "/etc/${config.services.kubernetes.pki.etcClusterAdminKubeconfig}";
      };

      environment.etc."kubernetes/redis/pod.json".source =
        pkgs.writeText
          "redis-pod.json" 
          (builtins.toJSON k8sResources.redisPod);

      environment.etc."kubernetes/redis/service.json".source =
        pkgs.writeText
          "redis-service.json"
          (builtins.toJSON redisService);
    };

  testScript = ''
    start_all()

    machine.wait_until_succeeds("kubectl get node $(hostname) | grep -w Ready")
    machine.wait_until_succeeds("kubectl apply -f /etc/kubernetes/redis/")
    machine.wait_until_succeeds("kubectl get pod redis | grep Running")
    out = machine.wait_until_succeeds("redis-cli -p 30000 ping")
    assert "PONG" in out
  '';
}
