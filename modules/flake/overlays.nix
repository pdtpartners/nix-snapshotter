{ self, inputs, ... }:
{
  # Provide overlay to add `nix-snapshotter`.
  flake.overlays.default = self: super:
    let
      nix-snapshotter = self.callPackage ../../package.nix {};

      # Depends on PR merged into main, but not yet in a release tag.
      # See: https://github.com/containerd/containerd/pull/9028
      containerd = super.containerd.overrideAttrs(o: {
        src = self.fetchFromGitHub {
          owner = "containerd";
          repo = "containerd";
          rev = "779875a057ff98e9b754371c193fe3b0c23ae7a2";
          hash = "sha256-sXMDMX0QPbnFvRYrAP+sVFjTI9IqzOmLnmqAo8lE9pg=";
        };
      });

    in {
      inherit 
        containerd
        nix-snapshotter
      ;

      k3s = (self.callPackage ./k3s {
        buildGoModule = self.buildGo120Module;
      }).k3s_1_27;
    };

  perSystem = { system, ... }: {
    _module.args.pkgs = import inputs.nixpkgs {
      inherit system;
      # Apply default overlay to provide nix-snapshotter for NixOS tests &
      # configurations.
      overlays = [ self.overlays.default ];
    };
  };
}
