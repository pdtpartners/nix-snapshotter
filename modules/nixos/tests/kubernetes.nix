{ pkgs, ... }:
let
  registryHost = "127.0.0.1";

  registryPort = 5000;

  redisImageName = "${registryHost}:${toString registryPort}/redis";

  redisNodePort = 30000;

  redisPod = pkgs.writeText "redis-pod.json" (builtins.toJSON {
    apiVersion = "v1";
    kind = "Pod";
    metadata = {
      name = "redis";
      labels.name = "redis";
    };
    spec.containers = [{
      name = "redis";
      image = redisImageName;
      args = ["--protected-mode" "no"];
      ports = [{
        name = "client";
        containerPort = 6379;
      }];
    }];
  });

  redisService = pkgs.writeText "redis-service.json" (builtins.toJSON {
    apiVersion = "v1";
    kind = "Service";
    metadata.name = "redis-service";
    spec = {
      type = "NodePort";
      selector.name = "redis";
      ports = [{
        name = "client";
        port = 6379;
        nodePort = redisNodePort;
      }];
    };
  });

in {
  nodes.machine = { config, nix-snapshotter-parts, ... }:
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

      redisImage = nix-snapshotter-parts.buildImage {
        name = redisImageName;
        tag = "latest";
        config.entrypoint = ["${pkgs.redis}/bin/redis-server"];
      };

    in {
      imports = [ ../nix-snapshotter.nix ];

      services.nix-snapshotter.enable = true;

      services.kubernetes = {
        roles = ["master" "node"];
        masterAddress = "localhost";
      };

      services.dockerRegistry = {
        enable = true;
        listenAddress = registryHost;
        port = registryPort;
      };

      environment.systemPackages = [
        (redisImage.copyToRegistry { plainHTTP = true; })
        pkgs.redis
        wrapKubectl
      ];
    };

  testScript = ''
    start_all()

    machine.wait_for_unit("docker-registry.service")
    machine.wait_for_open_port(${toString registryPort})
    machine.succeed("copy-to-registry")

    machine.wait_until_succeeds("kubectl get node machine | grep -w Ready")

    machine.wait_until_succeeds("kubectl create -f ${redisPod}")
    machine.wait_until_succeeds("kubectl create -f ${redisService}")

    machine.wait_until_succeeds("kubectl get pod redis | grep Running")
    machine.wait_until_succeeds("redis-cli -p ${toString redisNodePort} ping")
  '';
}
