{ lib, getSystem, withSystem, ... }:
let
  system = "x86_64-linux";

  packages = (getSystem system).packages;

  pkgs = withSystem system ({ pkgs, ...}: pkgs);

  pkgsModule = {
    _module.args.pkgs = lib.mkForce (pkgs.extend(self: super: {
      inherit (packages) nix-snapshotter;
    }));
    nixpkgs.hostPlatform = system;
  };

  vm = lib.nixosSystem {
    system = "x86_64-linux";
    modules = [
      pkgsModule
      ./nix-snapshotter.nix
      ./vm.nix
    ];
  };

in {
  flake.nixosModules.default = ./nix-snapshotter.nix;

  flake.nixosConfigurations = { inherit vm; };

  perSystem = { pkgs, ... }: {
    apps.vm = {
      type = "app";
      program = "${vm.config.system.build.vm}/bin/run-nixos-vm";
    };
  };
}
