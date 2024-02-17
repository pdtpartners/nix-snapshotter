{ self, lib, flake-parts-lib, ... }:
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

  config.perSystem = { config, pkgs, k8sResources, ... }:
    let
      pkgs' = pkgs.extend(self: super: {
        nix-snapshotter = super.nix-snapshotter.overrideAttrs(o: {
          # Build nix-snapshotter as a cover-instrumented binary.
          # See: https://go.dev/doc/build-cover
          preBuild = (o.preBuild or "") + ''
            buildFlagsArray+=(-cover)
          '';
        });
      });

      defaults = { config, ... }:
        let
          collectCoverage = {
            preStart = "mkdir -p $GOCOVERDIR";
            environment = { GOCOVERDIR = "/tmp/go-cover"; };
          };

        in { 
          imports = [
            self.nixosModules.default
          ];

          # Enable emitting coverage data for nix-snapshotter systemd units.
          config = lib.mkMerge [
            (lib.mkIf config.services.nix-snapshotter.enable {
              systemd.services.nix-snapshotter = collectCoverage;
            })
            (lib.mkIf config.services.nix-snapshotter.rootless.enable {
              systemd.user.services.nix-snapshotter = collectCoverage;
            })
          ];
        };

      evalTest = name: module:
        (lib.nixos.evalTest {
          imports = [
            { inherit name; }
            module
          ];
          hostPkgs = pkgs';
          node = {
            pkgs = pkgs';
            specialArgs = { inherit k8sResources; };
          };
          inherit defaults;
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
