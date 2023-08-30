{ self, lib, withSystem, ... }:
let
  nixosSystemFor = system: module:
    let
      pkgs = withSystem system ({ pkgs, ...}: pkgs);
      examples = withSystem system ({ examples, ...}: examples);

    in lib.nixosSystem {
      inherit system;
      modules = [
        {
          _module.args = {
            inherit examples;
            pkgs = lib.mkForce pkgs;
          };
        }
        self.nixosModules.default
        module
      ];
    };

in {
  /* NixOS module to provide nix-snapshotter systemd service.
   
    ```nix
    services.nix-snapshotter.enable = true;
    ```
  */
  flake.nixosModules = {
    default = {
      imports = [
        self.nixosModules.nix-snapshotter
        self.nixosModules.nix-snapshotter-rootless
      ];
    };

    nix-snapshotter = import ./nix-snapshotter.nix;
    nix-snapshotter-rootless = import ./nix-snapshotter-rootless.nix;
    containerd-rootless = import ./containerd-rootless.nix;
  };

  /* NixOS config for a VM to quickly try out nix-snapshotter.

     ```sh
     nixos-rebuild build-vm --flake .#vm
     ```
  */
  flake.nixosConfigurations.vm = nixosSystemFor "x86_64-linux" ./vm.nix;

  perSystem = { system, ... }: {
    /* A convenient `apps` target to run a NixOS VM to quickly try out
      nix-snaphotter without having `nixos-rebuild`.

      ```sh
      nix run .#vm
      ```
    */
    apps.vm = {
      type = "app";
      program = "${(nixosSystemFor system ./vm.nix).config.system.build.vm}/bin/run-nixos-vm";
    };

    # NixOS tests for nix-snapshotter.
    nixosTests.snapshotter = import ./tests/snapshotter.nix;
    nixosTests.kubernetes= import ./tests/kubernetes.nix;
  };
}
