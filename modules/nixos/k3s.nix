{ pkgs, ... }:
{
  services.k3s = {
    enable = true;
    extraFlags = toString [
      "--snapshotter nix"
    ];
  };

  systemd.services.k3s.path = with pkgs; [
    nix
  ];

  environment.sessionVariables = {
    KUBECONFIG = "/etc/rancher/k3s/k3s.yaml";
  };
}
