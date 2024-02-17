/*
  k3s-rootless configures k3s in rootless mode to use its embedded containerd
  with nix-snapshotter support.

  Note this is the only possible configuration for rootless mode. See
  tests/k3s.nix for more details.

*/
{ pkgs, lib, ... }:
{
  nodes.machine = {
    imports = [
      ../redis-spec.nix
    ];

    services.k3s.rootless = {
      enable = true;
      snapshotter = "nix";
    };

    environment.systemPackages = with pkgs; [
      kubectl
      redis
    ];

    users.users.alice = {
      uid = 1000;
      isNormalUser = true;
    };
  };

  testScript = { nodes, ... }:
    let
      sudo_su = lib.concatStringsSep " " [
        "KUBECONFIG=/home/alice/.kube/k3s.yaml"
        "sudo" "--preserve-env=KUBECONFIG" "-u" "alice"
      ];

    in ''
      start_all()

      machine.succeed("loginctl enable-linger alice")

      machine.wait_until_succeeds("${sudo_su} kubectl get node $(hostname) | grep -w Ready")
      machine.wait_until_succeeds("${sudo_su} kubectl apply -f /etc/kubernetes/redis/")
      machine.wait_until_succeeds("${sudo_su} kubectl get pod redis | grep Running")
      out = machine.wait_until_succeeds("${sudo_su} redis-cli -p 30000 ping")
      assert "PONG" in out
    '';
}
