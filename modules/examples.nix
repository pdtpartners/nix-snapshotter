{
  perSystem = { pkgs, nix-snapshotter-parts, ... }:
    let
      inherit (nix-snapshotter-parts)
        buildImage
      ;

    in {
      packages = rec {
        hello = buildImage {
          name = "docker.io/hinshun/hello";
          tag = "nix";
          config = {
            entrypoint = ["${pkgs.hello}/bin/hello"];
          };
        };

        redis = buildImage {
          name = "docker.io/hinshun/redis";
          tag = "nix";
          config = {
            entrypoint = [ "${pkgs.redis}/bin/redis-server" ];
          };
        };

        redisWithShell = buildImage {
          name = "docker.io/hinshun/redis-shell";
          tag = "nix";
          fromImage = redis;
          config = {
            entrypoint = [ "/bin/sh" ];
          };
          copyToRoot = pkgs.buildEnv {
            name = "system-path";
            pathsToLink = [ "/bin" ];
            paths = [
              pkgs.bashInteractive
              pkgs.coreutils
              pkgs.redis
            ];
          };
        };
      };
  };
}
