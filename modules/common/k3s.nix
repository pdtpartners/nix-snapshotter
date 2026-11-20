{ lib, ... }:
let
  inherit (lib)
    mkOption
    types
  ;

  options = {
    setEmbeddedContainerd = mkOption {
      type = types.bool;
      description = ''
        Configures CONTAINERD_ADDRESS, CONTAINERD_NAMESPACE,
        CONTAINERD_SNAPSHOTTER to target k3s' embedded containerd.
      '';
      default = false;
    };

    setKubeConfig = mkOption {
      type = types.bool;
      description = ''
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
      description = ''
        Specifies the containerd snapshotter for k3s' embedded containerd.
      '';
      default = "overlayfs";
    };

    pauseImage = mkOption {
      type = types.str;
      description = ''
        Sandbox (pause) image. Passed to k3s as --pause-image and, with
        virtualisation.containerd.k3sIntegration, used as the external
        containerd's sandbox image, so both use the same one. Must be fully
        qualified: containerd 2.3 looks the sandbox image up by exact name.
        The default is k3s' own, which its airgap images archive provides;
        with nix-snapshotter integration it's a preloaded nix:0 image.
      '';
      default = "docker.io/" + import ./k3s-pause-image.nix;
    };
  };

in {
  options.services.k3s = {
    inherit (options)
      setEmbeddedContainerd
      setKubeConfig
      snapshotter
      pauseImage
    ;

    lib = mkOption {
      type = types.attrs;
      description = "Common functions for k3s.";
      default = {
        inherit options;
      };
      internal = true;
    };
  };
}
