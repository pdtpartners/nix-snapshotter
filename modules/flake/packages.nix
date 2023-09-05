{ lib, ... }:
{
  perSystem = { pkgs, ... }: {
    packages = rec {
      inherit (pkgs) nix-snapshotter;
      default = nix-snapshotter;
    };

    devShells.default = pkgs.mkShell {
      packages = with pkgs; [
        containerd
        cri-tools
        delve
        gdb
        golangci-lint
        gopls
        gotools
        kind
        kubectl
        redis
        rootlesskit
        runc
        slirp4netns
        nerdctl
      ] ++ nix-snapshotter.nativeBuildInputs;
    };
  };
}
