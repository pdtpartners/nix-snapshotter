/* 
  k3s-external configures k3s to use an external containerd instead of its
  embedded containerd.

  This is more flexible as users can leverage the full set of options for the
  containerd & nix-snapshotter modules, whereas configuring them for the
  embedded containerd is less user friendly. In addition, each service will
  be its independent systemd unit.

  Note that rootless k3s cannot use an external containerd because doesn't
  provide a way to provision additional processes inside the namespaces
  managed by rootlesskit.

*/
{ pkgs, ... }:
{
  nodes.machine = {
    imports = [
      ../k3s.nix
      ../containerd.nix
      ../nix-snapshotter.nix
      ../redis-spec.nix
    ];

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
