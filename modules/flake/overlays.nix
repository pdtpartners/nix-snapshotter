{ self, inputs, ... }:
{
  # Provide overlay to add `nix-snapshotter`.
  flake.overlays.default = self: super: {
    nix-snapshotter = self.callPackage ../../package.nix {
      inherit (inputs) globset;
    };

    k3s = super.k3s_1_30.override {
      buildGoModule = args: super.buildGoModule (args // super.lib.optionalAttrs (args.pname != "k3s-cni-plugins" && args.pname != "k3s-containerd") {
        vendorHash = {
          "sha256-XtTahFaWnuHzKDI/U4d/j4C4gRxH163MCGEEM4hu/WM=" = "sha256-XuMP+ffwTdXKL9q9+ZJUQc5ghGEcdY9UdefjCD19OUE=";
          "sha256-Mj9Q3TgqZoJluG4/nyuw2WHnB3OJ+/mlV7duzWt1B1A=" = "sha256-9i0vY+CqrLDKYBZPooccX7OtFhS3//mpKTLntvPYDJo=";
        }.${args.vendorHash};
        patches = (args.patches or []) ++ [
          ./patches/k3s-nix-snapshotter.patch
        ];
      });
    };
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
