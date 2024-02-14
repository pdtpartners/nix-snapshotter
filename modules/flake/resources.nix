{
  perSystem = { pkgs, ... }:
    let
      redis = pkgs.nix-snapshotter.buildImage {
        name = "redis";
        resolvedByNix = true;
        config = {
          entrypoint = [ "${pkgs.redis}/bin/redis-server" ];
        };
      };

      k8sResources = {
        redisPod = {
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
        };

        redisService = {
          apiVersion = "v1";
          kind = "Service";
          metadata.name = "redis-service";
          spec = {
            # In rootless k3s, only LoadBalancer service ports are binded to host.
            type = "LoadBalancer";
            selector.name = "redis";
            ports = [{
              name = "client";
              # Exposed as localhost:30000
              port = 30000;
              targetPort = 6379;
            }];
          };
        };
      };

    in {
      # Load k8s resources into VM.
      _module.args = { inherit k8sResources; };

      inherit k8sResources;
    };
}
