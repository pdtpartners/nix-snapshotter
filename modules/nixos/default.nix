{ self, lib, withSystem, ... }:
let
  vmFor = system:
    let
      # NixOS systems need access to pkgs with nix-snapshotter, which is
      # provided by `pkgs'`.
      pkgs' = withSystem system ({ pkgs', ...}: pkgs');

    in lib.nixosSystem {
      inherit system;
      modules = [
        { _module.args.pkgs = lib.mkForce pkgs'; }
        self.nixosModules.default
        ./vm.nix
      ];
    };

in {
  /* NixOS module to provide nix-snapshotter systemd service.
   
    ```nix
    services.nix-snapshotter.enable = true;
    ```
  */
  flake.nixosModules.default = import ./nix-snapshotter.nix;

  /* NixOS config for a VM to quickly try out nix-snapshotter.

     ```sh
     nixos-rebuild build-vm --flake .#vm
     ```
  */
  flake.nixosConfigurations.vm = vmFor "x86_64-linux";

  perSystem = { system, ... }: {
    /* A convenient `apps` target to run a NixOS VM to quickly try out
      nix-snaphotter without having `nixos-rebuild`.

      ```sh
      nix run .#vm
      ```
    */
    apps.vm = {
      type = "app";
      program = "${(vmFor system).config.system.build.vm}/bin/run-nixos-vm";
    };

    # NixOS tests for nix-snapshotter.
    nixosTests.snapshotter = import ./tests/snapshotter.nix;
  };
}
