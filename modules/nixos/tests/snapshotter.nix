let
  registryHost = "127.0.0.1";

  registryPort = 5000;

  imageName = "${registryHost}:${toString registryPort}/hello";

in {
  nodes.machine = { pkgs, nix-snapshotter-parts, ... }:
    let
      hello = nix-snapshotter-parts.buildImage {
        name = imageName;
        tag = "latest";
        config.entrypoint = ["${pkgs.hello}/bin/hello"];
      };

    in {
      imports = [ ../nix-snapshotter.nix ];

      services.nix-snapshotter.enable = true;

      # Setup local registry for testing `buildImage` and `copyToRegistry`.
      services.dockerRegistry = {
        enable = true;
        listenAddress = registryHost;
        port = registryPort;
      };

      environment.systemPackages = [
        (hello.copyToRegistry { plainHTTP = true; })
        pkgs.nerdctl
      ];
    };

  testScript = ''
    machine.start(allow_reboot = True)

    machine.wait_for_unit("nix-snapshotter.service")
    machine.wait_for_unit("containerd.service")

    machine.wait_for_unit("docker-registry.service")
    machine.wait_for_open_port(${toString registryPort})

    with subtest("Nix-snapshotter image copied to local registry"):
        machine.succeed("copy-to-registry")

    with subtest("Nerdctl can pull and run nix-snapshotter image"):
        assert "Hello, world!" in machine.succeed("nerdctl --snapshotter nix run ${imageName}")
  '';
}
