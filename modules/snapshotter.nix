{
  perSystem = { pkgs, nix-snapshotter-parts, ... }:
    let
      inherit (nix-snapshotter-parts)
        nix-snapshotter
      ;

    in {
      packages = {
        inherit nix-snapshotter;
        default = nix-snapshotter;
      };

      devShells.default = pkgs.mkShell {
        packages = [
          pkgs.containerd
          pkgs.cri-tools
          pkgs.delve
          pkgs.gdb
          pkgs.golangci-lint
          pkgs.gopls
          pkgs.gotools
          pkgs.kind
          pkgs.kubectl
          pkgs.runc
        ] ++ nix-snapshotter.nativeBuildInputs;
      };
    };
}
