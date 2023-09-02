{ lib, ... }:
{
  perSystem = { pkgs, ... }: {
    packages = {
      inherit (pkgs) nix-snapshotter;
      default = pkgs.nix-snapshotter;
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
        pkgs.nerdctl
      ] ++ pkgs.nix-snapshotter.nativeBuildInputs;
    };
  };
}
