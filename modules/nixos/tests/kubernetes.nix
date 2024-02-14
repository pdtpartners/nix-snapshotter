{ pkgs, ... }:
{
  nodes = {
    kubernetes = { config, ... }:
      let
        cfg = config.services.kubernetes;

        wrapKubectl = pkgs.runCommand "wrap-kubectl" {
          nativeBuildInputs = [ pkgs.makeWrapper ];
        } ''
          mkdir -p $out/bin
          makeWrapper ${pkgs.kubernetes}/bin/kubectl \
            $out/bin/kubectl \
            --set KUBECONFIG "/etc/${cfg.pki.etcClusterAdminKubeconfig}"
        '';

      in {
        imports = [
          ../nix-snapshotter.nix
          ../redis-spec.nix
        ];

        services.nix-snapshotter.enable = true;

        services.kubernetes = {
          roles = ["master" "node"];
          masterAddress = "localhost";
          kubelet.extraOpts = "--image-service-endpoint unix:///run/nix-snapshotter/nix-snapshotter.sock";
        };

        environment.systemPackages = [
          pkgs.redis
          wrapKubectl
        ];
      };

    k3s = { ... }: {
      imports = [
        ../k3s.nix
        ../redis-spec.nix
      ];

      environment.systemPackages = [
        pkgs.redis
      ];
    };
  };

  testScript = ''
    start_all()

    def test_redis_service(machine):
      machine.wait_until_succeeds("kubectl get node $(hostname) | grep -w Ready")

      machine.wait_until_succeeds("kubectl apply -f /etc/kubernetes/redis/")

      machine.wait_until_succeeds("kubectl get pod redis | grep Running")
      out = machine.wait_until_succeeds("redis-cli -p 30000 ping")
      assert "PONG" in out

    # test_redis_service(kubernetes)
    test_redis_service(k3s)
  '';
}
