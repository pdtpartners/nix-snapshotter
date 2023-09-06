{
  perSystem = { pkgs, system, ... }: {
    _module.args.nix-snapshotter-parts = import ../. { inherit pkgs system; };
  };
}
