{ self, inputs, ... }:
{
  # Provide overlay to add `nix-snapshotter`.
  flake.overlays.default = self: super: {
    nix-snapshotter = self.callPackage ../../package.nix {};

    # Depends on PR merged into main, but not yet in a release tag.
    # See: https://github.com/containerd/containerd/pull/9028
    containerd = super.containerd.overrideAttrs(o: {
      src = self.fetchFromGitHub {
        inherit (o.src) owner repo;
        rev = "779875a057ff98e9b754371c193fe3b0c23ae7a2";
        hash = "sha256-sXMDMX0QPbnFvRYrAP+sVFjTI9IqzOmLnmqAo8lE9pg=";
      };
    });
  };

  perSystem = { system, ... }: {
    _module.args.pkgs = let
      # Creates a k3s overlay to get a recent version with the --image-service-endpoint flag
      nixpkgs-unstable = import inputs.nixpkgs-unstable {inherit system;};
      k3sOverlay = final: prev: {k3s = nixpkgs-unstable.k3s;};
    in import inputs.nixpkgs {
      inherit system;
      # Apply default overlay to provide nix-snapshotter for NixOS tests &
      # configurations.
      overlays = [ self.overlays.default k3sOverlay];
    };
  };
}
