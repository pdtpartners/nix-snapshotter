{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
    mkOption
    types
  ;

  cfg = config.services.nix-snapshotter;

  ns-lib = cfg.lib;

in {
  imports = [ ../common/nix-snapshotter-lib.nix ];

  options.services.nix-snapshotter = {
    inherit (ns-lib.options)
      configFile
      package
      path
      preloadContainerdImages
      setContainerdNamespace
      setContainerdSnapshotter
      settings
    ;

    enable = mkEnableOption "nix-snapshotter";
  };

  config = lib.mkIf cfg.enable (lib.mkMerge [
    {
      services.nix-snapshotter = {
        configFile =
          lib.mkOptionDefault
            (ns-lib.settingsFormat.generate "config.toml" cfg.settings);
      };

      environment.extraInit =
        (lib.optionalString cfg.setContainerdSnapshotter ''
          if [ -z "$CONTAINERD_SNAPSHOTTER" ]; then
            export CONTAINERD_SNAPSHOTTER="nix"
          fi
        '') +
        (lib.optionalString (cfg.setContainerdNamespace != "default") ''
          if [ -z "$CONTAINERD_NAMESPACE" ]; then
            export CONTAINERD_NAMESPACE="${cfg.setContainerdNamespace}"
          fi
        '');

      virtualisation.containerd = {
        enable = true;
        settings = ns-lib.mkContainerdSettings;
      };

      systemd.services.nix-snapshotter = lib.recursiveUpdate
        (ns-lib.convertServiceToNixOS ns-lib.mkNixSnapshotterService)
        {
          inherit (cfg) path;
          description = "nix-snapshotter - containerd snapshotter that understands nix store paths natively";
          wantedBy = [ "multi-user.target" ];
          after = [ "network.target" ];
          partOf = [ "containerd.service" ];
          serviceConfig.ExecStart = "${cfg.package}/bin/nix-snapshotter --config ${cfg.configFile}";
        };
    }
    (lib.mkIf (cfg.preloadContainerdImages != []) {
      systemd.services.preload-containerd-images = lib.recursiveUpdate
        (ns-lib.convertServiceToNixOS (ns-lib.mkPreloadContainerdImageService cfg))
        {
          description = "Preload images to containerd";
          wantedBy = [ "multi-user.target" ];
        };
    })
  ]);
}
