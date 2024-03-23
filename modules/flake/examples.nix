{
  perSystem = { lib, pkgs, ... }:
    let
      inherit (pkgs.nix-snapshotter)
        buildImage
      ;

      examples = rec {
        hello = buildImage {
          name = "ghcr.io/pdtpartners/hello";
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

      buildImages =
        lib.mapAttrs'
          (name: image: lib.nameValuePair ("image-" + name) image)
          examples;

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

      loadImages =
        let
          address = ''$XDG_RUNTIME_DIR/containerd/containerd.sock'';

        in lib.mapAttrs'
          (name: image: 
            lib.nameValuePair
              ("load-" + name)
              {
                type = "app";
                program = "${image.copyToContainerd { inherit address; }}/bin/copy-to-containerd";
              }
          )
          examples;

    in {
      # Load example images into VM.
      _module.args = { inherit examples; };

      apps = lib.mkMerge[ pushImages loadImages ];

      packages = buildImages;
    };
}
