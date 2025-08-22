{ self, lib, withSystem, ... }:
let
  nixosSystemFor = system: module:
    let
      pkgs = withSystem system ({ pkgs, ... }: pkgs);
      examples = withSystem system ({ examples, ... }: examples);
      k8sResources = withSystem system ({ k8sResources, ... }: k8sResources);

    in lib.nixosSystem {
      inherit system;
      specialArgs = { inherit lib; };
      modules = [
        {
          _module.args = {
            inherit examples k8sResources;
            pkgs = lib.mkForce pkgs;
          };
        }
        self.nixosModules.default
        module
      ];
    };

  vmApp = name: {
    type = "app";
    program = "${self.nixosConfigurations.${name}.config.system.build.vm}/bin/run-nixos-vm";
  };

in {
  /* NixOS module to provide nix-snapshotter systemd service.
   
    ```nix
    services.nix-snapshotter.enable = true;
    ```
  */
  flake.nixosModules = rec {
    default = {
      imports = [
        nix-snapshotter
        nix-snapshotter-rootless
        containerd
        containerd-rootless
        preload-containerd
        preload-containerd-rootless
        k3s
        k3s-rootless
      ];
    };

    nix-snapshotter = ./nix-snapshotter.nix;
    nix-snapshotter-rootless = ./nix-snapshotter-rootless.nix;
    containerd = ./containerd.nix;
    containerd-rootless = ./containerd-rootless.nix;
    preload-containerd = ./preload-containerd.nix;
    preload-containerd-rootless = ./preload-containerd-rootless.nix;
    k3s = ./k3s.nix;
    k3s-rootless = ./k3s-rootless.nix;
  };

  /* NixOS config for a VM to quickly try out nix-snapshotter.

     ```sh
     nixos-rebuild build-vm --flake .#vm
     ```
  */
  flake.nixosConfigurations = {
    vm = nixosSystemFor "x86_64-linux" ./vm.nix;
    vm-rootless = nixosSystemFor "x86_64-linux" ./vm-rootless.nix;
  };

  perSystem = { system, ... }: {
    /* A convenient `apps` target to run a NixOS VM to quickly try out
      nix-snaphotter without having `nixos-rebuild`.

      ```sh
      nix run .#vm
      ```
    */
    apps = {
      vm = vmApp "vm";
      vm-rootless = vmApp "vm-rootless";
    };

    # NixOS tests for nix-snapshotter.
    nixosTests.snapshotter = import ./tests/snapshotter.nix;
    nixosTests.snapshotter-1_7 = import ./tests/snapshotter-1_7.nix;
    nixosTests.push-n-pull = import ./tests/push-n-pull.nix;
    nixosTests.kubernetes = import ./tests/kubernetes.nix;
    nixosTests.k3s = import ./tests/k3s.nix;
    nixosTests.k3s-external = import ./tests/k3s-external.nix;
    nixosTests.k3s-rootless = import ./tests/k3s-rootless.nix;
    nixosTests.gvisor = import ./tests/gvisor.nix;
  };
}
