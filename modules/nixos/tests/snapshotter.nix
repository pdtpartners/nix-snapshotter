{ lib, ... }:
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
      ../nix-snapshotter.nix
    ];
    services.nix-snapshotter.setContainerdSnapshotter = true;
    services.nix-snapshotter.enable = true;
  };

  rootless = {
    imports = [
      base
      ../nix-snapshotter.nix
      ../nix-snapshotter-rootless.nix
    ];
    services.nix-snapshotter.rootless.setContainerdSnapshotter = true;
    services.nix-snapshotter.rootless.enable = true;

    users.users.alice = {
      uid = 1000;
      isNormalUser = true;
    };
  };

  both = { imports = [ rootful rootless ]; };

in {
  nodes = { inherit rootful rootless both; };

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

      setup(rootless)
      test_rootless(rootless)

      setup(both)
      test_rootful(both, "both")
      test_rootless(both, "both")
    '';
}
