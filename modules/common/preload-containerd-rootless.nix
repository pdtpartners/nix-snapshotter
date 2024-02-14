{ config, ... }:
let
  preload-lib = config.services.preload-containerd.lib;

in {
  imports = [
    ./preload-containerd.nix
  ];

  options.services.preload-containerd.rootless = {
    inherit (preload-lib.options)
      enable
      targets
    ;
  };
}
