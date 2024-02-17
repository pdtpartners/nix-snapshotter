{ config, pkgs, lib, ... }:
let
  registryHost = "127.0.0.1";

  registryPort = 5000;

  imageName = "${registryHost}:${toString registryPort}/hello";

  regularTag = "latest";
  regularImage = "${imageName}:${regularTag}";

  nixTag = "nix";
  nixImage = "${imageName}:${nixTag}";

  base = { pkgs, ... }:
    let
      helloTarball = pkgs.dockerTools.buildImage {
        name = imageName;
        tag = regularTag;
        config.entrypoint = ["${pkgs.hello}/bin/hello"];
      };

      hello-nix = pkgs.nix-snapshotter.buildImage {
        name = imageName;
        tag = nixTag;
        config.entrypoint = ["${pkgs.hello}/bin/hello"];
      };

    in {
      # Setup local registry for testing `buildImage` and `copyToRegistry`.
      services.dockerRegistry = {
        enable = true;
        listenAddress = registryHost;
        port = registryPort;
      };

      environment.variables = {
        HELLO_TARBALL = helloTarball;
      };

      environment.systemPackages = [
        (hello-nix.copyToRegistry { plainHTTP = true; })
        pkgs.nerdctl
      ];
    };

  rootful = {
    imports = [
      base
    ];

    virtualisation.containerd = {
      enable = true;
      nixSnapshotterIntegration = true;
    };

    services.nix-snapshotter = {
      enable = true;
    };
  };

  rootless = {
    imports = [
      base
    ];

    virtualisation.containerd.rootless = {
      enable = true;
      nixSnapshotterIntegration = true;
    };

    services.nix-snapshotter.rootless = {
      enable = true;
    };

    users.users.alice = {
      uid = 1000;
      isNormalUser = true;
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
  };

in {
  nodes = {
    inherit 
      rootful
      rootless
      external
    ;
  };

  testScript = { nodes, ... }:
    let
      user = nodes.rootless.users.users.alice;

      sudo_su = lib.concatStringsSep " " [
        "XDG_RUNTIME_DIR=/run/user/${toString user.uid}"
        "sudo" "--preserve-env=XDG_RUNTIME_DIR" "-u" "alice"
      ];

    in ''
      def setup(machine):
        machine.wait_for_unit("docker-registry.service")
        machine.wait_for_open_port(${toString registryPort})
        machine.succeed("copy-to-registry")

      def collect_coverage(machine):
        coverfiles = machine.succeed("ls /tmp/go-cover").split()
        for coverfile in coverfiles:
          machine.copy_from_vm(f"/tmp/go-cover/{coverfile}", f"build/go-cover/${config.name}-{machine.name}")

      def teardown_rootful(machine):
        machine.succeed("systemctl stop nix-snapshotter.service")
        collect_coverage(machine)

      def teardown_rootless(machine, user = "alice"):
        machine.succeed(f"systemctl --user --machine={user}@ stop nix-snapshotter.service")
        collect_coverage(machine)

      def wait_for_user_unit(machine, service, user = "alice"):
        machine.wait_until_succeeds(f"systemctl --user --machine={user}@ is-active {service}")

      def test_rootful(machine, name = "rootful"):
        machine.wait_for_unit("nix-snapshotter.service")
        machine.wait_for_unit("containerd.service")

        with subtest(f"{name}: Run regular container as root"):
          machine.succeed("nerdctl load < $HELLO_TARBALL")
          out = machine.succeed("nerdctl run --name hello ${regularImage}")
          assert "Hello, world!" in out
          machine.succeed("nerdctl ps -a | grep hello")
          machine.succeed("nerdctl rm hello")

        with subtest(f"{name}: Run nix container as root"):
          out = machine.succeed("nerdctl run --name hello ${nixImage}")
          assert "Hello, world!" in out
          machine.succeed("nerdctl ps -a | grep hello")
          machine.succeed("nerdctl rm hello")

      def test_rootless(machine, name = "rootless"):
        machine.succeed("loginctl enable-linger alice")
        wait_for_user_unit(machine, "nix-snapshotter.service")
        wait_for_user_unit(machine, "containerd.service")

        with subtest(f"{name}: Run regular container as user"):
          machine.succeed("${sudo_su} nerdctl load < $HELLO_TARBALL")
          out = machine.succeed("${sudo_su} nerdctl run --name hello ${regularImage}")
          assert "Hello, world!" in out
          machine.succeed("${sudo_su} nerdctl ps -a | grep hello")
          machine.succeed("${sudo_su} nerdctl rm hello")

        # TODO: Currently rootless nerdctl cannot pull images from 127.0.0.1,
        # because the pull operation occurs in rootlesskit's network namespace.
        #
        # With upcoming rootlesskit v2.0.0, there is a new flag `--detach-netns`
        # that when enabled, mounts the new netns to $ROOTLESSKIT_STATE_DIR/netns
        # and launches slirp4netns for that netns, but leaves containerd in host netns
        # with unshared userns and mountns.
        #
        # See:
        # - https://github.com/containerd/nerdctl/blob/main/docs/registry.md#accessing-127001-from-rootless-nerdctl
        # - https://github.com/containerd/nerdctl/issues/814
        # - https://github.com/rootless-containers/rootlesskit/pull/379
        # with subtest(f"{name}: Run nix container as user"):
        #   out = machine.succeed("${sudo_su} nerdctl run --name hello ${nixImage}")
        #   assert "Hello, world!" in out
        #   machine.succeed("${sudo_su} nerdctl ps -a | grep hello")
        #   machine.succeed("${sudo_su} nerdctl rm hello")

      start_all()

      setup(rootful)
      test_rootful(rootful)
      teardown_rootful(rootful)

      setup(rootless)
      test_rootless(rootless)
      teardown_rootless(rootless)

      setup(external)
      test_rootful(external)
      teardown_rootful(external)
    '';
}
