{ self, inputs, ... }:
{
  # Provide overlay to add `nix-snapshotter`.
  flake.overlays.default = self: super: {
    nix-snapshotter = self.callPackage ../package.nix {};

    # Apply patch on containerd to fix `ctr image import` issue not
    # working with remote snapshotters depending on `WithPullUnpack`
    # properties.
    # See: https://github.com/containerd/containerd/pull/9028
    containerd = super.containerd.overrideAttrs(o: {
      patches = (o.patches or []) ++ [
        ../script/patches/containerd-unpacker-wait.patch
      ];
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
