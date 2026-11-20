{
  perSystem = { pkgs, ... }: {
    packages = rec {
      inherit (pkgs)
        containerd
        containerd-1_7
        k3s
        nix-snapshotter
      ;

      default = nix-snapshotter;
    };

    # services.k3s.pauseImage defaults to k3s' own --pause-image default
    # (without nix-snapshotter); fail when the k3s package's default moves.
    checks.k3s-pause-image =
      let
        expected = import ../common/k3s-pause-image.nix;
      in pkgs.runCommand "k3s-pause-image" { } ''
        grep -A4 'Name: *"pause-image"' ${pkgs.k3s.src}/pkg/cli/cmds/agent.go \
          | grep -q 'Value: *"${expected}"' || {
          echo "k3s' --pause-image default is no longer ${expected}:"
          grep -A4 'Name: *"pause-image"' ${pkgs.k3s.src}/pkg/cli/cmds/agent.go
          exit 1
        }
        touch $out
      '';

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
