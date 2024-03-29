{ lib, examples, ... }:
{
  imports = [
    ./vm-common.nix
  ];

  # ROOTFUL
  #########

  users.users.root = {
    initialHashedPassword = null;
    password = "root";
  };

  services.k3s = {
    enable = true;
    setKubeConfig = true;
  };

  virtualisation.containerd = {
    enable = true;
    k3sIntegration = true;
    nixSnapshotterIntegration = true;
  };

  services.nix-snapshotter = {
    enable = true;
  };

  services.preload-containerd = {
    enable = true;
    targets = [{ archives = lib.attrValues examples; }];
  };
}
