{ config, pkgs, lib, ... }:

let
  registryConfig = {
    version =  "0.1";
    storage = {
      cache.blobdescriptor = "inmemory";
      filesystem.rootdirectory = "/var/lib/docker-registry";
    };
    http.addr = "0.0.0.0:5000";
  };

  configFile =
    pkgs.writeText
      "docker-registry-config.yml"
      (builtins.toJSON registryConfig);

  registry = pkgs.nix-snapshotter.buildImage {
    name = "ghcr.io/pdtpartners/registry";
    tag = "latest";
    config = {
      entrypoint = [ "${pkgs.docker-distribution}/bin/registry" ];
      cmd = [ "serve" configFile ];
    };
  };

  helloDockerTools = pkgs.dockerTools.buildImage {
    name = "localhost:5000/docker-tools/hello";
    tag = "latest";
    config.entrypoint = ["${pkgs.hello}/bin/hello"];
  };

  helloNixSnapshotter = pkgs.nix-snapshotter.buildImage {
    name = "localhost:5000/nix-snapshotter/hello";
    tag = "latest";
    config.entrypoint = ["${pkgs.hello}/bin/hello"];
  };

in {
  nodes = rec {
    rootful = {
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
            registry
            helloDockerTools
            helloNixSnapshotter
          ];
        }];
      };

      environment.systemPackages = [
        pkgs.nerdctl
      ];
    };

    rootless = {
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
            registry
            helloDockerTools
            helloNixSnapshotter
          ];
          address = "$XDG_RUNTIME_DIR/containerd/containerd.sock";
        }];
      };

      environment.systemPackages = [
        pkgs.nerdctl
      ];

      users.users.alice = {
        uid = 1000;
        isNormalUser = true;
      };

      environment.variables = {
        XDG_RUNTIME_DIR = "/run/user/1000";
      };
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
      def collect_coverage(machine):
        coverfiles = machine.succeed("ls /tmp/go-cover").split()
        for coverfile in coverfiles:
          machine.copy_from_vm(f"/tmp/go-cover/{coverfile}", f"build/go-cover/${config.name}-{machine.name}")

      def wait_for_unit(machine, service, user = "alice"):
        if "rootless" in machine.name:
          machine.wait_until_succeeds(f"systemctl --user --machine={user}@ is-active {service}")
        else:
          machine.wait_for_unit(service)

      def stop_unit(machine, service, user = "alice"):
        if "rootless" in machine.name:
          machine.succeed(f"systemctl --user --machine={user}@ stop {service}")
        else:
          machine.succeed(f"systemctl stop {service}")

      def test(machine, sudo_su = ""):
        wait_for_unit(machine, "nix-snapshotter.service")
        wait_for_unit(machine, "containerd.service")
        wait_for_unit(machine, "preload-containerd.service")

        machine.succeed(f"{sudo_su} nerdctl run -d -p 5000:5000 --name registry ghcr.io/pdtpartners/registry")

        with subtest(f"{machine.name}: Push container built with pkgs.dockerTools.buildImage"):
          machine.succeed(f"{sudo_su} nerdctl push localhost:5000/docker-tools/hello")
          machine.succeed(f"{sudo_su} nerdctl rmi localhost:5000/docker-tools/hello")

        with subtest(f"{machine.name}: Push container built with pkgs.nix-snapshotter.buildImage"):
          machine.succeed(f"{sudo_su} nerdctl push localhost:5000/nix-snapshotter/hello")
          machine.succeed(f"{sudo_su} nerdctl rmi localhost:5000/nix-snapshotter/hello")

        with subtest(f"{machine.name}: Pull container built with pkgs.dockerTools.buildImage"):
          machine.succeed(f"{sudo_su} nerdctl pull localhost:5000/docker-tools/hello")

        with subtest(f"{machine.name}: Pull container built with pkgs.nix-snapshotter.buildImage"):
          machine.succeed(f"{sudo_su} nerdctl pull localhost:5000/nix-snapshotter/hello")

        stop_unit(machine, "nix-snapshotter")
        collect_coverage(machine)

      start_all()

      test(rootful)
      test(rootless, "${sudo_su}")
    '';
}
