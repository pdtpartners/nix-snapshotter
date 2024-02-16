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

in {
  options.virtualisation.containerd = {
    inherit (options)
      k3sIntegration
      nixSnapshotterIntegration
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
          mkNixSnapshotterSettings
        ;
      };
      internal = true;
    };
  };
}
