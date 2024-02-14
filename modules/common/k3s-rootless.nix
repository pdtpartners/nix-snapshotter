{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
    mkOption
    mkPackageOption
    types
  ;

  cfg = config.services.k3s.rootless;

  k3s-lib = config.services.k3s.lib;

  mkRootlessK3sService = cfg: {
    Unit = {
      Description = "k3s - lightweight kubernetes (Rootless)";
      StartLimitBurst = "3";
      StartLimitInterval = "120s";
    };

    Install = {
      WantedBy = [ "default.target" ];
    };

    Service = {
      Type = "simple";
      Delegate = "yes";
      Restart = "always";
      RestartSec = "2";
      Environment = "PATH=${lib.makeBinPath cfg.path}";
      EnvironmentFile = cfg.environmentFile;
      ExecStart = lib.concatStringsSep " \\\n " (
        [
          "${pkgs.k3s}/bin/k3s server --rootless"
        ]
        ++ (lib.optional (cfg.configPath != null) "--config ${cfg.configPath}")
        ++ cfg.extraFlags
      );

      ExecReload = "${pkgs.procps}/bin/kill -s HUP $MAINPID";

      KillMode = "mixed";

      LimitNOFILE = "infinity";
      LimitNPROC = "infinity";
      LimitCORE = "infinity";
      TasksMax = "infinity";
    };
  };

in {
  imports = [
    ./k3s.nix
  ];

  options.services.k3s.rootless = {
    inherit (k3s-lib.options)
      setEmbeddedContainerd
      setKubeConfig
      snapshotter
    ;

    enable = mkEnableOption (lib.mdDoc "k3s");

    package = mkPackageOption pkgs "k3s" { };

    extraFlags = mkOption {
      type = types.listOf types.str;
      description = lib.mdDoc "Extra flags to pass to the k3s command.";
      default = [];
      example = [ "--no-deploy traefik" "--cluster-cidr 10.24.0.0/16" ];
    };

    path = mkOption {
      type = types.listOf types.path;
      description = lib.mdDoc ''
        Packages to be included in the PATH for k3s.
      '';
    };

    environmentFile = mkOption {
      type = types.nullOr types.path;
      description = lib.mdDoc ''
        File path containing environment variables for configuring the k3s
        service in the format of an EnvironmentFile. See systemd.exec(5).
      '';
      default = null;
    };

    configPath = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = lib.mdDoc ''
        File path containing the k3s YAML config. This is useful when the config is
        generated (for example on boot).
      '';
    };

    lib = mkOption {
      type = types.attrs;
      description = lib.mdDoc "Common functions for the k3s modules.";
      default = {
        inherit mkRootlessK3sService;
      };
      internal = true;
    };
  };

  config = lib.mkIf cfg.enable {
    services.k3s.rootless = lib.mkMerge [
      {
        path = with pkgs; [
          nerdctl
          slirp4netns
          # Need access to newuidmap from "/run/wrappers"
          "/run/wrappers"
        ];

        extraFlags = [ "--snapshotter ${cfg.snapshotter}" ];
      }
      (lib.mkIf (cfg.snapshotter == "nix") {
        path = [ pkgs.nix ];
      })
    ];
  };
}
