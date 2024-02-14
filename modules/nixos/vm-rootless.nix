{ lib, examples, ... }:
{
  imports = [
    ./vm-common.nix
  ];

  # ROOTLESS
  ##########

  users.users.rootless = {
    isNormalUser = true;
    extraGroups = [ "wheel" ];
    password = "rootless";
    group = "rootless";
  };
  users.groups.rootless = {};

  # Standalone k3s with embedded containerd and nix-snapshotter.
  services.k3s.rootless = {
    enable = true;
    setKubeConfig = true; 
    setEmbeddedContainerd = true;
    snapshotter = "nix";
  };

  services.preload-containerd.rootless = {
    enable = true;
    targets = [{
      archives = lib.attrValues examples;
      address = "$XDG_RUNTIME_DIR/k3s/containerd/containerd.sock";
      namespace = "k8s.io";
    }];
  };
}
