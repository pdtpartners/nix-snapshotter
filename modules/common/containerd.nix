{ pkgs, lib, ... }:
let
  inherit (lib)
    mkEnableOption
    mkOption
    types
  ;

  inherit (pkgs.go)
    GOOS
    GOARCH
  ;

  options = {
    k3sIntegration = mkEnableOption "K3s integration";

    nixSnapshotterIntegration = mkEnableOption "Nix snapshotter integration";

    gVisorIntegration = mkEnableOption "gVisor integration";

    defaultRuntime = mkOption {
      type = types.str;
      description = lib.mdDoc ''
        Configures the default CRI runtime for containerd.
      '';
      default = "runc";
    };

    path = mkOption {
      type = types.listOf types.path;
      description = lib.mdDoc ''
        Packages to be included in the PATH for containerd.
      '';
      default = [];
    };

    setAddress = mkOption {
      type = types.str;
      default = "/run/containerd/containerd.sock";
      description = lib.mdDoc ''
        Set the default containerd address via environment variable
        `CONTAINERD_ADDRESS`.
      '';
    };

    setNamespace = mkOption {
      type = types.str;
      default = "default";
      description = lib.mdDoc ''
        Set the default containerd namespace via environment variable
        `CONTAINERD_NAMESPACE`.
      '';
    };

    setSnapshotter = mkOption {
      type = types.str;
      default = "";
      description = lib.mdDoc ''
        Set the default containerd snapshotter via environment variable
        `CONTAINERD_SNAPSHOTTER`.
      '';
    };
  };

  mkNixSnapshotterSettings = {
    plugins."io.containerd.grpc.v1.cri".containerd = {
      snapshotter = "nix";
    };

    plugins."io.containerd.transfer.v1.local".unpack_config = [{
      platform = "${GOOS}/${GOARCH}";
      snapshotter = "nix";
    }];

    proxy_plugins.nix = {
      type = "snapshot";
      address = "/run/nix-snapshotter/nix-snapshotter.sock";
    };
  };

  mkSettings = cfg: {
    version = 2;
    plugins."io.containerd.grpc.v1.cri" = {
      cni = {
        conf_dir = lib.mkOptionDefault "/etc/cni/net.d";
        bin_dir = lib.mkOptionDefault "${pkgs.cni-plugins}/bin";
      };

      containerd = {
        default_runtime_name = cfg.defaultRuntime;

        runtimes.runc = {
          runtime_type = "io.containerd.runc.v2";
          options.SystemdCgroup = false;
        };
      };
    };
  };

  mkGVisorSettings = {
    plugins."io.containerd.grpc.v1.cri".containerd = {
      runtimes.runsc = {
        runtime_type = "io.containerd.runsc.v1";
      };
    };
  };

in {
  options.virtualisation.containerd = {
    inherit (options)
      k3sIntegration
      nixSnapshotterIntegration
      gVisorIntegration
      defaultRuntime
      path
      setAddress
      setNamespace
      setSnapshotter
    ;

    lib = mkOption {
      type = types.attrs;
      description = lib.mdDoc "Common functions for containerd.";
      default = {
        inherit
          options
          mkGVisorSettings
          mkNixSnapshotterSettings
          mkSettings
        ;
      };
      internal = true;
    };
  };
}
