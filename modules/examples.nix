{
  perSystem = {lib, pkgs, nix-snapshotter-parts, ... }:
    let
      inherit (nix-snapshotter-parts)
        buildImage
        copyToRegistry
      ;

      examples = rec {
        hello = buildImage {
          name = "ghcr.io/pdtpartners/hello";
          tag = "latest";
          config = {
            entrypoint = ["${pkgs.hello}/bin/hello"];
          };
        };

        redis = buildImage {
          name = "ghcr.io/pdtpartners/redis";
          tag = "latest";
          config = {
            entrypoint = [ "${pkgs.redis}/bin/redis-server" ];
          };
        };

        redisWithShell = buildImage {
          name = "ghcr.io/pdtpartners/redis-shell";
          tag = "latest";
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

      apps = 
        lib.mapAttrs'
          (name: image: 
            lib.nameValuePair
              ("push-" + name)
              {
                type = "app";
                program = "${image.copyToRegistry {}}/bin/copy-to-registry";
              }
              
          )
          examples;

    in {
      inherit apps;
      packages = { inherit (examples) hello redis redisWithShell; };
    };
}
