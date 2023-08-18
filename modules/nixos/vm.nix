{ pkgs, modulesPath, ... }:
{
  imports = [
    (modulesPath + "/virtualisation/qemu-vm.nix")
  ];

  nix-snapshotter.enable = true;

  nix.settings.experimental-features = [ "nix-command" "flakes" ];

  environment.systemPackages = with pkgs; [
    containerd
    cri-tools
    git
    nerdctl
    nix-snapshotter
    runc
  ];

  boot.kernelPackages = pkgs.linuxPackages_latest;

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
