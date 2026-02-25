{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
  ;

  cfg = config.services.nix-snapshotter;

  ns-lib = cfg.lib;

  settingsFormat = pkgs.formats.toml {};

in {
  imports = [
    ../common/nix-snapshotter.nix
    ./k3s.nix
  ];

  options.services.nix-snapshotter = {
    inherit (ns-lib.options)
      configFile
      package
      path
      extraFlags
      settings
    ;

    enable = mkEnableOption "nix-snapshotter";
  };

  config = lib.mkIf cfg.enable {
    services.nix-snapshotter.configFile =
      lib.mkOptionDefault
        (settingsFormat.generate "config.toml" cfg.settings);

    systemd.services.nix-snapshotter = lib.mkMerge [
      (ns-lib.convertServiceToNixOS ns-lib.mkNixSnapshotterService)
      {
        inherit (cfg) path;
        description = "nix-snapshotter - containerd snapshotter that understands nix store paths natively";
        wantedBy = [ "multi-user.target" ];
        after = [ "network.target" ];
        partOf = [ "containerd.service" ];
        serviceConfig.ExecStart = builtins.concatStringsSep " " ([
          "${cfg.package}/bin/nix-snapshotter"
          "--config ${cfg.configFile}"
        ] ++ cfg.extraFlags);
      }
    ];
  };
}
