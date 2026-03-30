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
          # The wrapper swallows PATH for some reason
          "${pkgs.k3s}/bin/.k3s-wrapped server --rootless"
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

    enable = mkEnableOption ("k3s");

    package = mkPackageOption pkgs "k3s" { };

    extraFlags = mkOption {
      type = types.listOf types.str;
      description = "Extra flags to pass to the k3s command.";
      default = [];
      example = [ "--no-deploy traefik" "--cluster-cidr 10.24.0.0/16" ];
    };

    path = mkOption {
      type = types.listOf types.path;
      description = ''
        Packages to be included in the PATH for k3s.
      '';
    };

    environmentFile = mkOption {
      type = types.nullOr types.path;
      description = ''
        File path containing environment variables for configuring the k3s
        service in the format of an EnvironmentFile. See systemd.exec(5).
      '';
      default = null;
    };

    configPath = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = ''
        File path containing the k3s YAML config. This is useful when the config is
        generated (for example on boot).
      '';
    };

    lib = mkOption {
      type = types.attrs;
      description = "Common functions for the k3s modules.";
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
          runc         # OCI runtime (wrapper adds it, but we bypass wrapper)
          slirp4netns
          util-linux   # for nsenter
          iproute2     # for ip
          iptables     # for firewalling
          # Need access to newuidmap from "/run/wrappers/bin"
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
