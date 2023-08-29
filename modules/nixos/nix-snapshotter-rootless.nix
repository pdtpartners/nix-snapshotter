{ options, config, pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
    mkOption
    mkPackageOptionMD
    types
  ;

  inherit (config.virtualisation.containerd.rootless)
    nsenter
  ;

  cfg = config.services.nix-snapshotter.rootless;

  ns-lib = config.services.nix-snapshotter.lib;

  settingsFormat = pkgs.formats.toml {};

  configFile = settingsFormat.generate "config.toml" cfg.settings;

in {
  imports = [ ./containerd-rootless.nix ];

  options.services.nix-snapshotter.rootless = {
    inherit (options.services.nix-snapshotter)
      path
      settings
      setContainerdSnapshotter
      setContainerdNamespace
      preloadContainerdImages
    ;

    enable = mkOption {
      type = types.bool;
      default = false;
      description = lib.mdDoc ''
        This option enables nix-snapshotter and containerd in rootless mode.
        To interact with the containerd daemon, one needs to set
        {command}`CONTAINERD_ADDRESS=$XDG_RUNTIME_DIR/containerd/containerd.sock`.
      '';
    };

    package = mkPackageOptionMD pkgs "nix-snapshotter" { };
  };

  config = lib.mkIf cfg.enable (lib.mkMerge [
    {
      services.nix-snapshotter = lib.mkDefault {
        inherit (cfg)
          setContainerdSnapshotter
          setContainerdNamespace
        ;
      };

      virtualisation.containerd.rootless = {
        enable = true;

        # Configure containerd with nix-snapshotter.
        settings = ns-lib.baseContainerdSettings;

        bindMounts = {
          "$XDG_RUNTIME_DIR/nix-snapshotter".mountPoint = "/run/nix-snapshotter";
          "$XDG_DATA_HOME/nix-snapshotter".mountPoint = "/var/lib/containerd/io.containerd.snapshotter.v1.nix";
        };
      };

      systemd.user.services.nix-snapshotter = {
        inherit (cfg) path;
        description = "nix-snapshotter - containerd snapshotter that understands nix store paths natively (Rootless)";
        wantedBy = [ "default.target" ];
        partOf = [ "containerd.service" ];
        after = [ "containerd.service" ];
        serviceConfig = lib.mkMerge [
          ns-lib.baseServiceConfig
          {
            ExecStart = "${nsenter}/bin/containerd-nsenter ${cfg.package}/bin/nix-snapshotter --log-level debug --config ${configFile}";
          }
        ];
      };
    }
    (lib.mkIf (cfg.preloadContainerdImages != []) {
      systemd.user.services.preload-containerd-images = lib.mkMerge [
        (ns-lib.mkPreloadContainerdImageService {
          archives = cfg.preloadContainerdImages;
          namespace = cfg.setContainerdNamespace;
        })
        {
          description = "Preload images to containerd (Rootless)";
          wantedBy = [ "default.target" ];
          environment.CONTAINERD_ADDRESS = "%t/containerd/containerd.sock";
        }
      ];
    })
  ]);
}
