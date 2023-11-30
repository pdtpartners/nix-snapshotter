{ lib, config, pkgs, modulesPath, examples, ... }:
let
  preloadContainerdImages = lib.attrValues examples;

in {
  imports = [
    # Import qemu-vm directly to avoid using vmVariant since this config
    # is only intended to be used as a VM. Using vmVariant will emit assertion
    # errors regarding `fileSystems."/"` and `boot.loader.grub.device`.
    (modulesPath + "/virtualisation/qemu-vm.nix")
    ./kubernetes.nix
    # ./k3s.nix
    ./redis-spec.nix
  ];

  # Enable rootful & rootless nix-snapshotter. This also starts rootful &
  # rootless containerd respectively.
  services.nix-snapshotter = {
    enable = true;
    setContainerdSnapshotter = true;
    inherit preloadContainerdImages;
  };

  services.nix-snapshotter.rootless = {
    enable = true;
    inherit preloadContainerdImages;
  };

  nix.settings.experimental-features = [ "nix-command" "flakes" ];

  environment.systemPackages = with pkgs; [
    bat
    containerd
    cri-tools
    git
    jq
    kubectl
    nerdctl
    nix-snapshotter
    redis
    tree
    vim
  ];

  users.users = {
    root = {
      initialHashedPassword = null;
      password = "root";
    };
    rootless = {
      isNormalUser = true;
      extraGroups = [ "wheel" ];
      password = "rootless";
      group = "rootless";
    };
  };

  users.groups.rootless = {};

  virtualisation = {
    memorySize = 4096;
    cores = 4;
    graphics = false;
    diskImage = null;
  };

  services.openssh.enable = true;

  networking.firewall.allowedTCPPorts = [ 22 ];

  system.stateVersion = "23.05";
}
