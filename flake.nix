{
  description = "Containerd snapshotter using nix store directly";

  inputs = {
    nixpkgs.url = "nixpkgs/nixos-22.11";
  };

  outputs = { self, nixpkgs }:
    let
      # to work with older version of flakes
      lastModifiedDate = self.lastModifiedDate or self.lastModified or "19700101";

      # Generate a user-friendly version number.
      version = builtins.substring 0 8 lastModifiedDate;

      # System types to support.
      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];

      # Helper function to generate an attrset '{ x86_64-linux = f "x86_64-linux"; ... }'.
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      overlays = [
        (self: super: {
          # Containerd needs a patch for nix-snapshotter to work. Remove this
          # overlay when upstream PR has been merged.
          # See: https://github.com/containerd/containerd/pull/7840
          containerd = super.containerd.overrideAttrs(o: {
            src = builtins.fetchTarball {
              url = "https://github.com/hinshun/containerd/archive/0ab5f5392b488ccf5a59e6b4e297077fcca46639.tar.gz";
              sha256 = "sha256-vOKAqC9zaP/cP8SP8+3xv33hXT7UAjl7qRZzOEKj2ls=";
            };
          });
        })
      ];

      # Nixpkgs instantiated for supported system types.
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system overlays; });

    in
    rec {
      # Provide some binary packages for selected system types.
      packages = forAllSystems (system:
        let
          pkgs = nixpkgsFor.${system};
          utils = (import ./.) { inherit pkgs system; };
        in
        rec {
          nix-snapshotter = utils.nix-snapshotter;

          hello = utils.buildImage {
            name = "docker.io/hinshun/hello";
            tag = "nix";
            config = {
              entrypoint = ["${pkgs.hello}/bin/hello"];
            };
          };

          redis = utils.buildImage {
            name = "docker.io/hinshun/redis";
            tag = "nix";
            config = {
              entrypoint = [ "${pkgs.redis}/bin/redis-server" ];
            };
          };

          redisWithShell = utils.buildImage {
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
        });

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgsFor.${system};
        in
        {
          default = pkgs.stdenv.mkDerivation {
            name = "nix-snapshotter";
            buildInputs = [
              pkgs.containerd
              pkgs.cri-tools
              pkgs.delve
              pkgs.gdb
              pkgs.gotools
              pkgs.kind
              pkgs.kubectl
              pkgs.runc
            ] ++ packages.${system}.nix-snapshotter.nativeBuildInputs;
          };
        });

      defaultPackage = forAllSystems (system: self.packages.${system}.nix-snapshotter);
    };
}
