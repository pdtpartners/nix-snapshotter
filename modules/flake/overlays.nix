{ self, inputs, ... }:
{
  # Provide overlay to add `nix-snapshotter`.
  flake.overlays.default = self: super: {
    nix-snapshotter = self.callPackage ../../package.nix { };

    # Depends on PR merged into main, but not yet in nixpkgs. Included in 2.0.0-beta.1 and later.
    # See: https://github.com/containerd/containerd/pull/9028
    containerd = super.containerd.overrideAttrs (o: {
      src = self.fetchFromGitHub {
        inherit (o.src) owner repo;
        rev = "779875a057ff98e9b754371c193fe3b0c23ae7a2";
        hash = "sha256-sXMDMX0QPbnFvRYrAP+sVFjTI9IqzOmLnmqAo8lE9pg=";
      };
    });


    # Fixes https://github.com/pdtpartners/nix-snapshotter/issues/102 due to 23.11 only supporting v1.27.6 while we need v1.27.7. 
    # Remove once we upgrade to the next stable release.
    k3s = super.k3s.overrideAttrs (oldAttrs: {
      k3sRepo = super.fetchgit {
        url = "https://github.com/k3s-io/k3s";
        rev = "v1.27.9+k3s1";
        sha256 = "sha256-Zr9Zp9pi7S3PCTveiuSb0RebiGZrxxKC+feTAWO47Js=";
      };
    });
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
