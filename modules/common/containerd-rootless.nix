{ config, pkgs, lib, ... }:
let
  inherit (lib)
    mkOption
    mkPackageOptionMD
    types
  ;

  cfg = config.virtualisation.containerd.rootless;

  settingsFormat = pkgs.formats.toml {};

  configFile = settingsFormat.generate "containerd.toml" cfg.settings;

  containerdConfigChecked = pkgs.runCommand "containerd-config-checked.toml" {
    nativeBuildInputs = [ pkgs.containerd ];
  } ''
    containerd -c ${configFile} config dump >/dev/null
    ln -s ${configFile} $out
  '';

  bindMountOpts = {
    options = {
      mountPoint = mkOption {
        type = types.str;
        example = "/run/containerd";
        description = lib.mdDoc "Mount point in the rootless mount namespace.";
      };
    };
  };

  nsenter = pkgs.writeShellApplication {
    name = "containerd-nsenter";
    runtimeInputs = [
      pkgs.util-linux
      "/run/current-system/sw"
    ];
    text = ''
      pid=$(<"$XDG_RUNTIME_DIR/containerd-rootless/child_pid")
      exec nsenter \
        --no-fork \
        --preserve-credentials \
        -m -n -U \
        -t "$pid" \
        -- "$@"
    '';
  };

  makeProg = args: pkgs.substituteAll (args // {
    inherit (pkgs) runtimeShell;
    dir = "bin";
    isExecutable = true;
  });

  mkRootlessContainerdService = cfg:
    let
      containerdArgs = lib.concatStringsSep " " (lib.cli.toGNUCommandLine {} cfg.args);

      containerd-rootless = makeProg {
        name = "containerd-rootless";
        src = ./containerd-rootless.sh;
        inherit containerdArgs;
        path = lib.makeBinPath [
          containerd-rootless-child
          pkgs.bash
          pkgs.iproute2
          pkgs.libselinux
          pkgs.rootlesskit
          pkgs.slirp4netns
          pkgs.util-linux
          # Need access to newuidmap from "/run/wrappers"
          "/run/wrappers"
        ];
      };

      mountSources = lib.concatStringsSep " " (
        builtins.map
          (source: ''"${source}"'')
          (lib.attrNames cfg.bindMounts)
      );

      mountPoints = lib.concatStringsSep " " (
        builtins.map
          (opts: ''"${opts.mountPoint}"'')
          (lib.attrValues cfg.bindMounts)
      );

      containerd-rootless-child = makeProg {
        name = "containerd-rootless-child";
        src = ./containerd-rootless-child.sh;
        inherit mountSources mountPoints;
        path = lib.makeBinPath [
          cfg.package
          pkgs.coreutils
          pkgs.iptables
          pkgs.kmod
          pkgs.runc
          # Mount only works inside user namespaces from "/run/current-system/sw"
          # See: https://github.com/NixOS/nixpkgs/issues/42117#issuecomment-872029461
          "/run/current-system/sw"
        ];
      };

    in {
      Unit = {
        Description = "containerd - container runtime (Rootless)";

        # containerd-rootless doesn't support running as root.
        ConditionUser = "!root";
        StartLimitBurst = "16";
        StartLimitInterval = "120s";
      };

      Install = {
        WantedBy = [ "default.target" ];
      };

      Service = {
        Type = "notify";
        Delegate = "yes";
        Restart = "always";
        RestartSec = "10";
        ExecStart = "${containerd-rootless}/bin/containerd-rootless";
        ExecReload = "${pkgs.procps}/bin/kill -s HUP $MAINPID";

        StateDirectory = "containerd";
        RuntimeDirectory = "containerd";
        RuntimeDirectoryPreserve = "yes";

        # Don't kill child processes like containerd-shim.
        KillMode = "process"; 

        # Allow process in pid namespace to notify systemd.
        NotifyAccess = "all";

        # Having non-zero Limit*s causes performance problems due to accounting
        # overhead in the kernel. We recommend using cgroups to do
        # container-local accounting.
        #
        # Limits adopted from upstream.
        # See: https://github.com/containerd/containerd/blob/c3f3cad287fb53793c83b8d83397ef1187ad27a1/containerd.service
        LimitNOFILE = "infinity";
        LimitNPROC = "infinity";
        LimitCORE = "infinity";
        TasksMax = "infinity";
        OOMScoreAdjust="-999";
      };
    };

in {
  options.virtualisation.containerd.rootless = {
    settings = lib.mkOption {
      type = settingsFormat.type;
      default = {};
      description = lib.mdDoc ''
        Verbatim lines to add to containerd.toml
      '';
    };

    args = lib.mkOption {
      type = types.attrsOf types.str;
      default = {};
      description = lib.mdDoc "extra args to append to the containerd cmdline";
    };

    enable = mkOption {
      type = types.bool;
      default = false;
      description = lib.mdDoc ''
        This option enables containerd in a rootless mode, a daemon that
        manages linux containers. To interact with the daemon, one needs to set
        {command}`CONTAINERD_ADDRESS=unix://$XDG_RUNTIME_DIR/containerd/containerd.sock`.
      '';
    };

    package = mkPackageOptionMD pkgs "containerd" { };

    setSocketVariable = mkOption {
      type = types.bool;
      default = false;
      description = lib.mdDoc ''
        Point {command}`CONTAINERD_ADDRESS` to rootless containerd for normal
        users by default.
      '';
    };

    bindMounts = lib.mkOption {
      type = types.attrsOf (types.submodule bindMountOpts);
      example = lib.literalExpression ''
        {
          "$XDG_RUNTIME_DIR/containerd".mountPoint = "/run/containerd";
        }
      '';
      description = lib.mdDoc ''
        A list of bind mounts inside the mount namespace. Since paths like
        `/run` are copied up by rootlesskit, this allows sockets inside the
        mount namespace to be exposed in host directories like
        $XDG_RUNTIME_DIR.
      '';
    };

    nsenter = mkOption {
      type = types.package;
      description = lib.mdDoc ''
        Defines a package to nsenter into containerd's fakeroot setup by
        rootlesskit.
      '';
    };

    lib = mkOption {
      description = lib.mdDoc "Common functions for the containerd modules.";
      default = {
        inherit mkRootlessContainerdService;
      };
      type = types.attrs;
      internal = true;
    };
  };

  config = lib.mkIf cfg.enable {
    virtualisation.containerd.rootless = {
      inherit nsenter;

      args = {
        config = toString containerdConfigChecked;
      };

      settings = {
        version = 2;
        plugins."io.containerd.grpc.v1.cri" = {
         cni.bin_dir = lib.mkOptionDefault "${pkgs.cni-plugins}/bin";
        };
      };

      bindMounts = {
        "$XDG_RUNTIME_DIR/containerd".mountPoint = "/run/containerd";
        "$XDG_DATA_HOME/containerd".mountPoint = "/var/lib/containerd";
        "$XDG_DATA_HOME/cni".mountPoint = "/var/lib/cni";
        "$XDG_CONFIG_HOME/cni".mountPoint = "/etc/cni";
      };
    };
  };
}
