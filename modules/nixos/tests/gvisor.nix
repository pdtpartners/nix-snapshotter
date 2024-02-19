{ lib, pkgs, ... }:
let
  redis = pkgs.nix-snapshotter.buildImage {
    name = "ghcr.io/pdtpartners/redis";
    tag = "latest";
    copyToRoot = [
      pkgs.util-linux
    ];
    config = {
      Entrypoint = [ "${pkgs.redis}/bin/redis-server" ];
      Cmd = [ "--protected-mode" "no" ];
    };
  };

  common = {
    environment.systemPackages = [
      pkgs.nerdctl
      pkgs.redis
    ];

    nix.settings.experimental-features = [ "nix-command" ];
  };

in {
  nodes = {
    rootful = {
      imports = [
        common
      ];

      virtualisation.containerd = {
        enable = true;
        nixSnapshotterIntegration = true;
        gVisorIntegration = true;
      };

      services.nix-snapshotter = {
        enable = true;
      };

      services.preload-containerd = {
        enable = true;
        targets = [{
          archives = [ redis ];
        }];
      };
    };

    rootless = {
      imports = [
        common
      ];

      virtualisation.containerd.rootless = {
        enable = true;
        nixSnapshotterIntegration = true;
        gVisorIntegration = true;
      };

      services.nix-snapshotter.rootless = {
        enable = true;
      };

      services.preload-containerd.rootless = {
        enable = true;
        targets = [{
          archives = [ redis ];
          address = "$XDG_RUNTIME_DIR/containerd/containerd.sock";
        }];
      };

      users.users.alice = {
        uid = 1000;
        isNormalUser = true;
      };

      environment.variables = {
        XDG_RUNTIME_DIR = "/run/user/1000";
      };
    };
  };

  testScript = { nodes, ... }:
    let
      sudo_su = lib.concatStringsSep " " [
        "sudo"
        "--preserve-env=XDG_RUNTIME_DIR,CONTAINERD_ADDRESS,CONTAINERD_SNAPSHOTTER"
        "-u"
        "alice"
      ];

    in ''
      def test(machine, sudo_su = ""):
        if sudo_su == "":
          machine.wait_for_unit("nix-snapshotter.service")
          machine.wait_for_unit("containerd.service")
          machine.wait_for_unit("preload-containerd.service")
        else:
          machine.succeed("loginctl enable-linger alice")
          wait_for_user_unit(machine, "nix-snapshotter.service")
          wait_for_user_unit(machine, "containerd.service")
          wait_for_user_unit(machine, "preload-containerd.service")

        with subtest(f"{machine.name}: Run redis using runtime runsc"):
          machine.succeed(f"{sudo_su} nerdctl run -d --name redis --runtime runsc -p 30000:6379 --cap-add syslog ghcr.io/pdtpartners/redis")

        with subtest(f"{machine.name}: Ensure that gVisor is active"):
          out = machine.succeed(f"{sudo_su} nerdctl exec redis dmesg | grep -i gvisor")
          assert "Starting gVisor" in out

        with subtest(f"{machine.name}: Ensure that redis is healthy"):
          out = machine.wait_until_succeeds(f"{sudo_su} redis-cli -p 30000 ping")
          assert "PONG" in out

      def wait_for_user_unit(machine, service, user = "alice"):
        machine.wait_until_succeeds(f"systemctl --user --machine={user}@ is-active {service}")

      start_all()
      test(rootful)
      test(rootless, "${sudo_su}")
    '';
}
