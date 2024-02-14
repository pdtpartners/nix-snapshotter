{
  /* Home manager module to provide rootless nix-snapshotter systemd service.
   
    ```nix
    services.nix-snapshotter.rootless.enable = true;
    ```
  */
  flake.homeModules = rec {
    default = {
      imports = [
        nix-snapshotter-rootless
        containerd-rootless
      ];
    };
    nix-snapshotter-rootless = ./nix-snapshotter-rootless.nix;
    containerd-rootless = ./containerd-rootless.nix;
    preload-containerd-rootless = ./preload-containerd-rootless.nix;
    k3s-rootless = ./k3s-rootless.nix;
  };
}
