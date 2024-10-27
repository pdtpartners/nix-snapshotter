{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
    mkOption
    mkPackageOption
    types
  ;

  inherit (config.virtualisation.containerd.rootless)
    nsenter
  ;

  cfg = config.services.buildkit.rootless;

  settingsFormat = pkgs.formats.toml {};

  buildkitService = {
    Unit = {
      Description = "BuildKit - a toolkit for converting source code to build artifacts (Rootless)";
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
      Environment = "PATH=${lib.makeBinPath cfg.path}";

      ExecStart = "${nsenter}/bin/containerd-nsenter ${cfg.package}/bin/buildkitd --rootless";

      StateDirectory = "buildkit";
      RuntimeDirectory = "buildkit";
      RuntimeDirectoryPreserve = "yes";
    };
  };

in {
  imports = [
    ../common/containerd-rootless.nix
    ../common/nix-snapshotter-rootless.nix
  ];

  options.services.buildkit.rootless = {
    enable = mkEnableOption (lib.mdDoc "buildkit");

    configFile = mkOption {
      type = types.nullOr types.path;
      description = lib.mdDoc ''
       Path to nix-snapshotter config file.
       Setting this option will override any configuration applied by the
       settings option.
      '';
    };

    package = mkPackageOption pkgs "buildkit" { };

    path = mkOption {
      type = types.listOf types.path;
      description = lib.mdDoc ''
        Packages to be included in the PATH for k3s.
      '';
    };

    settings = mkOption {
      type = settingsFormat.type;
      default = {};
      description = lib.mdDoc ''
        Verbatim lines to add to config.toml
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    home.sessionVariablesExtra = ''
      if [ -z "$BUILDKIT_HOST" ]; then
        export BUILDKIT_HOST="unix://$XDG_RUNTIME_DIR/buildkit/buildkitd.sock"
      fi
    '';

    services.buildkit.rootless = {
      configFile =
        lib.mkOptionDefault
          (settingsFormat.generate "config.toml" cfg.settings);

      path = with pkgs; [
        git
      ];

      settings = {
        worker.containerd = with config.virtualisation.containerd.rootless; {
          enabled = true;
          address = setAddress;
          namespace = setNamespace;
        };
      };
    };

    systemd.user.services.buildkit = buildkitService;
  };
}
