{
  perSystem = { pkgs, nix-snapshotter-parts, ... }:
    let
      inherit (nix-snapshotter-parts)
        buildImage
        copyToRegistry
      ;
      hello = buildImage {
        name = "ghcr.io/pdtpartners/hello";
        tag = "nix";
        config = {
          entrypoint = ["${pkgs.hello}/bin/hello"];
        };
      };

      redis = buildImage {
        name = "ghcr.io/pdtpartners/redis";
        tag = "nix";
        config = {
          entrypoint = [ "${pkgs.redis}/bin/redis-server" ];
        };
      };

      redisWithShell = buildImage {
        name = "ghcr.io/pdtpartners/redis-shell";
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
    in {
      packages = { inherit hello redis redisWithShell;};
      apps.copyHelloToRegistry = {
        type = "app";
        program = "${hello.copyToRegistry {}}/bin/copy-to-registry";
      };
    };
}
