{ lib, config, pkgs, modulesPath, ... }:
{
  imports = [
    # Import qemu-vm directly to avoid using vmVariant since this config
    # is only intended to be used as a VM. Using vmVariant will emit assertion
    # errors regarding `fileSystems."/"` and `boot.loader.grub.device`.
    (modulesPath + "/virtualisation/qemu-vm.nix")
    ./kubernetes-startup.nix
  ];

  # Enable rootful & rootless nix-snapshotter. This also starts rootful &
  # rootless containerd respectively.
  services.nix-snapshotter = {
    enable = true;
    rootless.enable = true;
    setContainerdSnapshotter = true;
  };

  # Provision single node kubernetes listening on localhost.
  services.kubernetes = {
    roles = ["master" "node"];
    masterAddress = "localhost";
  };

  # Allow non-root "admin" user to just use `kubectl`.
  services.certmgr.specs.clusterAdmin.private_key.owner = "admin";
  environment.sessionVariables = {
    KUBECONFIG = "/etc/${config.services.kubernetes.pki.etcClusterAdminKubeconfig}";
  };

  # Provide an example kubernetes config for redis using a nix-snapshotter
  # image.
  environment.etc."kubernetes/redis.yaml".source = ../../script/k8s/redis.yaml;

  nix.settings.experimental-features = [ "nix-command" "flakes" ];

  environment.systemPackages = with pkgs; [
    containerd
    cri-tools
    git
    kubectl
    nerdctl
    nix-snapshotter
    redis
    vim
  ];

  users.users = {
    admin = {
      isNormalUser = true;
      extraGroups = [ "wheel" ];
      password = "admin";
      group = "admin";
    };
  };

  virtualisation = {
    memorySize = 2048;
    cores = 4;
    graphics = false;
    diskImage = null;
  };

  services.openssh.enable = true;

  networking.firewall.allowedTCPPorts = [ 22 ];

  system.stateVersion = "23.05";
}
