{ pkgs, ... }:
let
  redis = pkgs.nix-snapshotter.buildImage {
    name = "redis";
    resolvedByNix = true;
    config = {
      entrypoint = [ "${pkgs.redis}/bin/redis-server" ];
    };
  };

  redisPod = pkgs.writeText "redis-pod.json" (builtins.toJSON {
    apiVersion = "v1";
    kind = "Pod";
    metadata = {
      name = "redis";
      labels.name = "redis";
    };
    spec.containers = [{
      inherit (redis) name image;
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
        nodePort = 30000;
      }];
    };
  });

in {
  # Provide an example kubernetes config for redis using a nix-snapshotter
  # image.
  environment.etc."kubernetes/redis/pod.json".source = redisPod;
  environment.etc."kubernetes/redis/service.json".source = redisService;
}
