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

    services.k3s = {
      enable = true;
      setKubeConfig = true;
      snapshotter = "nix";
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
