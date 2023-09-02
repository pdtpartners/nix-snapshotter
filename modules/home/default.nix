{
  /* Home manager module to provide rootless nix-snapshotter systemd service.
   
    ```nix
    services.nix-snapshotter.rootless.enable = true;
    ```
  */
  flake.homeModules = rec {
    default = nix-snapshotter-rootless;
    nix-snapshotter-rootless = ./nix-snapshotter-rootless.nix;
    containerd-rootless = ./containerd-rootless.nix;
  };
}
