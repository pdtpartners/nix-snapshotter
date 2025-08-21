/*
  k3s configures k3s to use its embedded containerd with nix-snapshotter
  support.

  This is the simplest configuration as it's a single systemd unit. However
  less flexible than the setup in tests/k3s-external.nix.

*/
{ pkgs, ... }:
{
  nodes.machine = {
    imports = [
      ../redis-spec.nix
    ];

    # Runs out of space on default 1024.
    virtualisation.diskSize = 2048;

    services.k3s = {
      enable = true;
      setKubeConfig = true;
      snapshotter = "nix";
      # Force k3s listen on IPv4 localhost otherwise it'll bind on a IPv6
      # address.
      moreFlags = [
        "--node-ip=127.0.0.1"
      ];
    };

    environment.systemPackages = with pkgs; [
      redis
    ];
  };

  testScript = ''
    start_all()

    machine.wait_until_succeeds("kubectl get node $(hostname) | grep -w Ready")
    machine.wait_until_succeeds("kubectl apply -f /etc/kubernetes/redis/")
    machine.wait_until_succeeds("kubectl get pod redis | grep Running")
    out = machine.wait_until_succeeds("redis-cli -p 30000 ping")
    assert "PONG" in out
  '';
}
