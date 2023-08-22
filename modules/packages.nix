{
  perSystem = { pkgs, nix-snapshotter-parts, ... }:
    let
      inherit (nix-snapshotter-parts)
        nix-snapshotter
      ;

      nerdctl = pkgs.nerdctl.overrideAttrs(o: {
        patches = [
          ../script/rootless/nerdctl-ocihook.patch
        ];
      });

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
          pkgs.rootlesskit
          pkgs.runc
          pkgs.slirp4netns
          nerdctl
        ] ++ nix-snapshotter.nativeBuildInputs;
      };
    };
}
