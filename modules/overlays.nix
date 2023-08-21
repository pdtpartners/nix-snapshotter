{ self, ... }:
{
  # Provide overlay to add `nix-snapshotter`.
  flake.overlays.default = self: super:
    let parts = import ../. { pkgs = super; system = self.system; };
    in { inherit (parts) nix-snapshotter; };

  perSystem = { pkgs, ... }: {
    # Define new module arg pkgs' with default overlay applied to provide
    # nix-snapshotter for NixOS tests/configurations.
    _module.args.pkgs' = pkgs.extend(self.overlays.default);
  };
}
