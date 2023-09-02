{
  perSystem = { lib, pkgs, ... }:
    let
      inherit (pkgs.nix-snapshotter)
        buildImage
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

      pushImages =
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

      buildImages =
        lib.mapAttrs'
          (name: image: lib.nameValuePair ("image-" + name) image)
          examples;

    in {
      # Load example images into VM.
      _module.args = { inherit examples; };

      apps = pushImages;

      packages = buildImages;
    };
}
