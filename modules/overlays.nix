{ self, inputs, ... }:
{
  # Provide overlay to add `nix-snapshotter`.
  flake.overlays.default = self: super:
    let parts = import ../. { pkgs = super; system = self.system; };
    in { inherit (parts) nix-snapshotter; };

  perSystem = { system, ... }: {
    _module.args.pkgs = import inputs.nixpkgs {
      inherit system;
      overlays = [
        # Apply default overlay to provide nix-snapshotter for NixOS tests &
        # configurations.
        self.overlays.default
        (self: super: {
          # Apply patch on containerd to fix `ctr image import` issue not
          # working with remote snapshotters depending on `WithPullUnpack`
          # properties.
          # See: https://github.com/containerd/containerd/pull/9028
          containerd = super.containerd.overrideAttrs(o: {
            patches = (o.patches or []) ++ [
              ../script/patches/containerd-unpacker-wait.patch
            ];
          });
        })
      ];
    };
  };
}
