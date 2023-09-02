{ config, lib, ... }:
let
  inherit (lib.home-manager)
    convertServiceToNixOS
  ;

  cfg = config.virtualisation.containerd.rootless;

  proxyEnv = config.networking.proxy.envVars;

in {
  imports = [ ../common/containerd-rootless.nix ];

  config = lib.mkIf cfg.enable {
    environment.extraInit = lib.optionalString cfg.setSocketVariable ''
      if [ -z "$CONTAINERD_ADDRESS" -a -n "$XDG_RUNTIME_DIR" ]; then
        export CONTAINERD_ADDRESS="$XDG_RUNTIME_DIR/containerd/containerd.sock"
      fi
    '';

    systemd.user.services.containerd = lib.recursiveUpdate
      (convertServiceToNixOS (cfg.lib.mkRootlessContainerdService cfg))
      { environment = proxyEnv; };
  };
}
