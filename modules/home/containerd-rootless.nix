{ config, lib, ... }:
let
  cfg = config.virtualisation.containerd.rootless;

  ns-lib = config.services.nix-snapshotter.lib;

in {
  imports = [ ../common/containerd-rootless.nix ];

  config = lib.mkIf cfg.enable {
    home.sessionVariablesExtra = lib.optionalString cfg.setSocketVariable ''
      if [ -z "$CONTAINERD_ADDRESS" -a -n "$XDG_RUNTIME_DIR" ]; then
        export CONTAINERD_ADDRESS="$XDG_RUNTIME_DIR/containerd/containerd.sock"
      fi
    '';

    systemd.user.services.containerd = cfg.lib.mkRootlessContainerdService cfg;
  };
}
