{ lib, flake-parts-lib, ... }:
let
  inherit (lib)
    mkOption
    types
  ;

  inherit (flake-parts-lib)
    mkPerSystemOption
  ;

in {
  options.perSystem = mkPerSystemOption {
    _file = ./nixosTests.nix;

    options.nixosTests = mkOption {
      type = types.attrsOf types.deferredModule;
      default = { };
    };
  };

  config.perSystem = { config, pkgs, nix-snapshotter-parts, ... }:
    let
      evalTest = name: module:
        (lib.nixos.evalTest {
          imports = [
            { inherit name; }
            module
          ];
          hostPkgs = pkgs;
          node.pkgs = pkgs;
          extraBaseModules = {
            _module.args = { inherit nix-snapshotter-parts; };
          };
        }).config.result;

      testRigs = lib.mapAttrs (name: module: evalTest name module) config.nixosTests;

      /* For each nixosTest, add an `apps` target that allows the use of
         `machine.shell_interact()` for developing tests.
        
         ```sh
         nix run .#test-<name> -L
         ```
      */
      apps =
        lib.mapAttrs'
          (name: testRig:
            lib.nameValuePair
              ("test-" + name)
              {
                type = "app";
                program = "${testRig.driver}/bin/nixos-test-driver";
              }
          )
          testRigs;

    in {
      inherit apps;
    };
}
