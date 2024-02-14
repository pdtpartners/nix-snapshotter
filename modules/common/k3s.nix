{ lib, ... }:
let
  inherit (lib)
    mkOption
    types
  ;

  options = {
    setEmbeddedContainerd = mkOption {
      type = types.bool;
      description = lib.mdDoc ''
        Configures CONTAINERD_ADDRESS, CONTAINERD_NAMESPACE,
        CONTAINERD_SNAPSHOTTER to target k3s' embedded containerd.
      '';
      default = false;
    };

    setKubeConfig = mkOption {
      type = types.bool;
      description = lib.mdDoc ''
        Configures KUBECONFIG environment variable to default kubectl to point
        to k3s.
      '';
      default = false;
    };

    snapshotter = mkOption {
      type = types.enum [
        "overlayfs"
        "fuse-overlayfs"
        "stargz"
        "nix"
      ];
      description = lib.mdDoc ''
        Specifies the containerd snapshotter for k3s' embedded containerd.
      '';
      default = "overlayfs";
    };
  };

in {
  options.services.k3s = {
    inherit (options)
      setEmbeddedContainerd
      setKubeConfig
      snapshotter
    ;

    lib = mkOption {
      type = types.attrs;
      description = lib.mdDoc "Common functions for k3s.";
      default = {
        inherit options;
      };
      internal = true;
    };
  };
}
