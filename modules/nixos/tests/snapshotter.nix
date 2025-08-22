{ lib
, pkgs
, ...
}@args:
let
  common = if args ? common then args.common else {};

  helloDrvFile = pkgs.nix-snapshotter.buildImage {
    name = "ghcr.io/nix-snapshotter/hello-world";
    tag = "latest";
    config.entrypoint = [
      (pkgs.writeShellScript "hello-world" ''
        #!${pkgs.runtimeShell}
        echo "Hello, world!"
      '')
    ];
  };

  redisDockerTools = pkgs.dockerTools.buildImage {
    name = "ghcr.io/docker-tools/redis";
    tag = "latest";
    config = {
      Entrypoint = [ "${pkgs.redis}/bin/redis-server" ];
      Cmd = [ "--protected-mode" "no" ];
    };
  };

  redisNixSnapshotter = pkgs.nix-snapshotter.buildImage {
    name = "ghcr.io/nix-snapshotter/redis";
    tag = "latest";
    config = {
      Entrypoint = [ "${pkgs.redis}/bin/redis-server" ];
      Cmd = [ "--protected-mode" "no" ];
    };
  };

in {
  nodes = rec {
    rootful = {
      imports = [ common ];

      virtualisation.containerd = {
        enable = true;
        nixSnapshotterIntegration = true;
      };

      services.nix-snapshotter = {
        enable = true;
      };

      services.preload-containerd = {
        enable = true;
        targets = [{
          archives = [
            helloDrvFile
            redisDockerTools
            redisNixSnapshotter
          ];
        }];
      };

      environment.systemPackages = with pkgs; [
        nerdctl
        redis
      ];
    };

    rootless = {
      imports = [ common ];

      virtualisation.containerd.rootless = {
        enable = true;
        nixSnapshotterIntegration = true;
      };

      services.nix-snapshotter.rootless = {
        enable = true;
      };

      services.preload-containerd.rootless = {
        enable = true;
        targets = [{
          archives = [
            helloDrvFile
            redisDockerTools
            redisNixSnapshotter
          ];
          address = "$XDG_RUNTIME_DIR/containerd/containerd.sock";
        }];
      };

      environment.systemPackages = with pkgs; [
        nerdctl
        redis
      ];

      users.users.alice = {
        uid = 1000;
        isNormalUser = true;
        linger = true;
      };

      environment.variables = {
        XDG_RUNTIME_DIR = "/run/user/1000";
      };
    };

    external = {
      imports = [
        rootful
      ];

      virtualisation.containerd = {
        settings.external_builder = pkgs.writeScript "external-builder.sh" ''
          ${pkgs.nix}/bin/nix build --out-link $1 $2
        '';
      };

      nix.settings.experimental-features = [ "nix-command" ];
    };

  };

  testScript =
    let
      sudo_su = lib.concatStringsSep " " [
        "sudo"
        "--preserve-env=XDG_RUNTIME_DIR,CONTAINERD_ADDRESS,CONTAINERD_SNAPSHOTTER"
        "-u"
        "alice"
      ];

    in ''
      def wait_for_unit(machine, service, user = "alice"):
        if "rootless" in machine.name:
          machine.wait_until_succeeds(f"systemctl --user --machine={user}@ is-active {service}")
        else:
          machine.wait_for_unit(service)

      def test(machine, sudo_su = ""):
        wait_for_unit(machine, "nix-snapshotter.service")
        wait_for_unit(machine, "containerd.service")
        wait_for_unit(machine, "preload-containerd.service")

        with subtest(f"{machine.name}: Run container with an executable outPath"):
          out = machine.succeed(f"{sudo_su} nerdctl run ghcr.io/nix-snapshotter/hello-world")
          assert "Hello, world!" in out

        with subtest(f"{machine.name}: Run container with CNI built with pkgs.dockerTools.buildImage"):
          machine.succeed(f"{sudo_su} nerdctl run -d -p 30000:6379 ghcr.io/docker-tools/redis")
          out = machine.wait_until_succeeds(f"{sudo_su} redis-cli -p 30000 ping")
          assert "PONG" in out

        with subtest(f"{machine.name}: Run container with CNI built with pkgs.nix-snapshotter.buildImage"):
          machine.succeed(f"{sudo_su} nerdctl run -d -p 30001:6379 ghcr.io/nix-snapshotter/redis")
          out = machine.wait_until_succeeds(f"{sudo_su} redis-cli -p 30001 ping")
          assert "PONG" in out

      start_all()

      test(rootful)
      test(rootless, "${sudo_su}")
      test(external)
    '';
}
