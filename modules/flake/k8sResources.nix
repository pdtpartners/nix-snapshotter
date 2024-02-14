{ lib, flake-parts-lib, ... }:
let
  inherit (lib)
    mkOption
    types
  ;

  inherit (flake-parts-lib)
    mkTransposedPerSystemModule
  ;

in mkTransposedPerSystemModule {
  name = "k8sResources";

  option = mkOption {
    type = types.lazyAttrsOf (types.attrsOf types.raw);
    default = { };
    description = ''
      An attribute set of kubernetes resources.
    '';
  };

  file = ./k8sResources.nix;
}
