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
{ config, pkgs, ... }:
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
      # Force k3s listen on IPv4 localhost otherwise it'll bind on a IPv6
      # address.
      moreFlags = [
        "--node-ip=127.0.0.1"
      ];
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

    # Copy out test coverage data.
    machine.succeed("systemctl stop nix-snapshotter.service")
    coverfiles = machine.succeed("ls /tmp/go-cover").split()
    for coverfile in coverfiles:
      machine.copy_from_vm(f"/tmp/go-cover/{coverfile}", f"build/go-cover/${config.name}-{machine.name}")
  '';
}
