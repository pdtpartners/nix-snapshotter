{ self, inputs, ... }:
{
  # Provide overlay to add `nix-snapshotter`.
  flake.overlays.default = self: super:
    let
      nix-snapshotter = self.callPackage ../../package.nix {};

      containerd = super.containerd.overrideAttrs(o: rec {
        version = "1.7.14";
        src = self.fetchFromGitHub {
          owner = "containerd";
          repo = "containerd";
          rev = "v${version}";
          hash = "sha256-okTz2UCF5LxOdtLDBy1pN2to6WHi+I0jtR67sn7Qrbk=";
        };
        patches = (o.patches or []) ++ [
          # See: https://github.com/containerd/containerd/pull/9864
          ./patches/containerd-import-compressed.patch
        ];
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
