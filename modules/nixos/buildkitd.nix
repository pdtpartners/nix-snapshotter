{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
    mkOption
    mkPackageOption
    types
  ;

  cfg = config.services.buildkitd;

  ns-lib = config.services.nix-snapshotter.lib;

  settingsFormat = pkgs.formats.toml {};

  buildkitdService = {
    Unit = {
      After = [ "containerd.service" ];
      PartOf = [ "containerd.service" ];
    };

    Install = {
      WantedBy = [ "default.target" ];
    };

    Service = {
      Type = "simple";
      KillMode = "mixed";
      Restart = "always";
      RestartSec = "2";

      ExecStart = "${cfg.package}/bin/buildkitd --config ${cfg.configFile}";

      StateDirectory = "buildkit";
      RuntimeDirectory = "buildkit";
      RuntimeDirectoryPreserve = "yes";
    };
  };

in {
  options.services.buildkitd = {
    enable = mkEnableOption (lib.mdDoc "buildkitd");

    configFile = mkOption {
      type = types.nullOr types.path;
      description = lib.mdDoc ''
       Path to buildkitd config file.
       Setting this option will override any configuration applied by the
       settings option.
      '';
    };

    package = mkPackageOption pkgs "buildkit" { };

    listenOptions =
      mkOption {
        type = types.listOf types.str;
        default = ["/run/buildkit/buildkitd.sock"];
        description = ''
          A list of unix and tcp buildkitd should listen to. The format follows
          ListenStream as described in systemd.socket(5).
        '';
      };

    path = mkOption {
      type = types.listOf types.path;
      description = lib.mdDoc ''
        Packages to be included in the PATH for buildkitd.
      '';
    };

    settings = mkOption {
      type = settingsFormat.type;
      default = {};
      description = lib.mdDoc ''
        Verbatim lines to add to buildkitd.toml
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    environment.extraInit = ''
      if [ -z "$BUILDKIT_HOST" ]; then
        export BUILDKIT_HOST="unix:///run/buildkit/buildkitd.sock"
      fi
    '';

    ids.gids.buildkitd = 889; # unused gid.

    users.groups.buildkitd.gid = config.ids.gids.buildkitd;

    services.buildkitd = {
      configFile =
        lib.mkOptionDefault
          (settingsFormat.generate "config.toml" cfg.settings);

      path = with pkgs; [
        git
      ];

      settings = {
        grpc.gid = config.ids.gids.buildkitd;

        worker.containerd = with config.virtualisation.containerd; {
          enabled = true;
          address = setAddress;
          namespace = setNamespace;
        };
      };
    };

    systemd.sockets.buildkitd = {
      description = "Buildkitd Socket for the API";
      wantedBy = [ "sockets.target" ];
      socketConfig = {
        ListenStream = cfg.listenOptions;
        SocketMode = "0660";
        SocketUser = "root";
        SocketGroup = "buildkitd";
      };
    };

    systemd.services.buildkitd = lib.mkMerge [
      (ns-lib.convertServiceToNixOS buildkitdService)
      {
        inherit (cfg) path;
        description = "BuildKit - a toolkit for converting source code to build artifacts";
      }
    ];
  };
}
