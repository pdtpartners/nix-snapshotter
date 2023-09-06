<div align="center">

# nix-snapshotter

[![Go Reference][go-reference-badge]][go-reference]
[![ci][ci-badge]][ci]
[![Go Report Card][go-report-card-badge]][go-report-card]

Brings native understanding of Nix packages to [containerd](https://github.com/containerd/containerd).

[Key features](#key-features) •
[Getting started](#getting-started) •
[Installation](#installation) •
[Architecture](docs/architecture.md) •
[Contributing](CONTRIBUTING.md)

</div>

## Key features

- Rather than download image layers, packages come directly from the Nix store.
- Packages can be fetched from a binary cache or built on the fly if necessary.
- Backwards compatible with existing non-Nix images.
- Nix layers can be interleaved with normal layers.
- Allows Kubernetes to resolve image manifests from Nix too.
- Enables fully-declarative Kubernetes resources, including image
  specification.
- Run pure Nix images without a Docker Registry at all, if you wish.

## Getting started

![Demo](docs/demo.gif)

The easiest way to try this out is run a NixOS VM with everything
pre-configured.

> **Note**
> You'll need [Nix][nix] installed with [flake support][nix-flake] and [unified CLI][nix-command] enabled,
> which comes pre-enabled with [Determinate Nix Installer][nix-installer].
>
> <details>
> <summary>Trying without Nix installed</summary>
>
> If you have [docker][docker] or another OCI runtime installed, you can run
> `docker run --rm -it nixpkgs/nix-flakes`:
>
> ```sh
> nix run github:pdtpartners/nix-snapshotter#vm
> ```
> </details>

```sh
nix run "github:pdtpartners/nix-snapshotter#vm"
nixos login: root # (Ctrl-a then x to quit)
Password: root

# Running `pkgs.hello` image with nix-snapshotter
nerdctl run ghcr.io/pdtpartners/hello

# Running `pkgs.redis` image with kubernetes & nix-snapshotter
kubectl apply -f /etc/kubernetes/redis/

# Wait a few seconds... 
watch kubectl get pods

# And a kubernetes service will be ready to forward port 30000 to the redis
# pod, so you can test it out with a `ping` command
redis-cli -p 30000 ping
```

Or you can try running in rootless mode:

```sh
nix run ".#vm"
nixos login: rootless # (Ctrl-a then x to quit)
Password: rootless

# Running pkgs.hello image with nix-snapshotter
nerdctl run ghcr.io/pdtpartners/hello

# Rootless kubernetes not supported yet.
# See: https://github.com/k3s-io/k3s/pull/8279
```

## Installation

[NixOS][nixos] and [Home Manager][home-manager] modules are provided for
easy installation.

> **Note**
> Requires at least nixpkgs 23.05+

- **Home Manager**

  <details>
  <summary>Flake</summary>

  ```nix
  {
    inputs = {
      nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
      home-manager = {
        url = "github:nix-community/home-manager";
        inputs.nixpkgs.follows = "nixpkgs";
      };
      nix-snapshotter = {
        url = "github:pdtpartners/nix-snapshotter";
        inputs.nixpkgs.follows = "nixpkgs";
      };
    };

    outputs = { nixpkgs, home-manager, nix-snapshotter, ... }: {
      homeConfigurations.myuser = home-manager.lib.homeManagerConfiguration {
        pkgs = import nixpkgs { system = "x86_64-linux"; };
        };
        modules = [
          {
            home = {
              username = "myuser";
              homeDirectory = "/home/myuser";
              stateVersion = "23.11";
            };

            programs.home-manager.enable = true;

            # Let home-manager automatically start systemd user services.
            # Will eventually become the new default.
            systemd.user.startServices = "sd-switch";
          }
          ({ pkgs, ... }: {
            # (1) Import home-manager module.
            imports = [ nix-snapshotter-rootless ];

            # (2) Add overlay.
            nixpkgs.overlays = [ nix-snapshotter.overlays.default ];

            # (3) Enable service.
            services.nix-snapshotter.rootless = {
              enable = true;
              setContainerdSnapshotter = true;
            };

            # (4) Add a containerd CLI like nerdctl.
            home.packages = [ pkgs.nerdctl ];
          })
        ];
      };
    };
  }
  ```
  </details>

  <details>
  <summary>Non-flake</summary>

  ```nix
  { pkgs, ... }:
  let
    nix-snapshotter = import (
      builtins.fetchTarball "https://github.com/pdtpartners/nix-snapshotter/archive/main.tar.gz"
    );

  in {
    imports = [
      ./hardware-configuration.nix
      # (1) Import home-manager module.
      nix-snapshotter.nixosModules.default
    ];

    # (2) Add overlay.
    nixpkgs.overlays = [ nix-snapshotter.overlays.default ];

    # (3) Enable service.
    services.nix-snapshotter = {
      enable = true;
      setContainerdSnapshotter = true;
    };

    # (4) Add a containerd CLI like nerdctl.
    environment.systemPackages = [ pkgs.nerdctl ];
  }
  ```
  </details>

- **NixOS**

  <details>
  <summary>Flake</summary>

  ```nix
  {
    inputs = {
      nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
      nix-snapshotter = {
        url = "github:pdtpartners/nix-snapshotter";
        inputs.nixpkgs.follows = "nixpkgs";
      };
    };

    outputs = { nixpkgs, nix-snapshotter, ... }: {
      nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [
          ./hardware-configuration.nix
          ({ pkgs, ... }: {
            # (1) Import nixos module.
            imports = [ nix-snapshotter.nixosModules.default ];

            # (2) Add overlay.
            nixpkgs.overlays = [ nix-snapshotter.overlays.default ];

            # (3) Enable service.
            services.nix-snapshotter = {
              enable = true;
              setContainerdSnapshotter = true;
            };

            # (4) Add a containerd CLI like nerdctl.
            environment.systemPackages = [ pkgs.nerdctl ];
          })
        ];
      };
    };
  }
  ```
  </details>

  <details>
  <summary>Non-flake</summary>

  ```nix
  { pkgs, ... }:
  let
    nix-snapshotter = import (
      builtins.fetchTarball "https://github.com/pdtpartners/nix-snapshotter/archive/main.tar.gz"
    );

  in {
    imports = [
      ./hardware-configuration.nix
      # (1) Import home-manager module.
      nix-snapshotter.nixosModules.default
    ];

    # (2) Add overlay.
    nixpkgs.overlays = [ nix-snapshotter.overlays.default ];

    # (3) Enable service.
    services.nix-snapshotter = {
      enable = true;
      setContainerdSnapshotter = true;
    };

    # (4) Add a containerd CLI like nerdctl.
    environment.systemPackages = [ pkgs.nerdctl ];
  }
  ```
  </details>

- **Manual**

  See the [manual installation docs][manual-install].

## Contributing

Pull requests are welcome for any changes. Consider opening an issue to discuss
larger changes first to get feedback on the idea.

Please read [CONTRIBUTING](CONTRIBUTING.md) for development tips and
more details on contributing guidelines.

## FAQ

1. What's the difference between this and [pkgs.dockerTools.buildImage][dockerTools]?

<details>
<summary>Answer</summary>

The upstream `buildImage` streams Nix packages into tarballs, compresses them
and pushes them to an OCI registry. Since there is a limit to number of layers
in an image, a heuristic is used to put popular packages together. There is
large amount of duplication between your Nix binary cache and the Docker
Registry tarballs, and even between images that share packages as the layers may
duplicate common packages due to the heuristic-based layering strategy.

With `pkgs.nix-snapshotter.buildImage`, containerd natively understand Nix
packages, so everything is pulled at package granularity without the layer
limit. This means all the container content is either already in your host nix
store or fetched from your Nix binary cache.
</details>

2. What's the difference between this and [Nixery][nixery]?

<details>
<summary>Answer</summary>

Nixery exposes an API (in the form of an OCI registry) to dynamically build
Nix-based images, but still uses the same layering strategy as upstream's
`pkgs.dockerTools.buildImage` (see above). So Nixery suffers from the same
inefficiency in duplication. However, Nixery can totally start building
nix-snapshotter images so we can have a Docker Registry that can dynamically
build native Nix images.

</details>

3. What's the difference between this and a nix-in-docker?

<details>
<summary>Answer</summary>

If you run nix inside a container (e.g. `nixos/nix` or `nixpkgs/nix-flake`)
then you are indeed fetching packages using the Nix store. However, each
container will have its own Nix store instead of de-duplicating at the host
level.

nix-snapshotter is intended to live on the host system (sibling to containerd
and/or kubelet) so that multiple containers running different images can share
the underlying packages from the same Nix store.

</details>

## License

The source code developed for nix-snapshotter is licensed under MIT License.

This project also contains modified portions of other projects that are
licensed under the terms of Apache License 2.0. See [NOTICE](NOTICE) for more
details.

[ci-badge]: https://github.com/pdtpartners/nix-snapshotter/actions/workflows/ci.yml/badge.svg
[ci]: https://github.com/pdtpartners/nix-snapshotter/actions?query=workflow%3ACI
[docker]: https://www.docker.com/
[dockerTools]: https://nixos.org/manual/nixpkgs/stable/#ssec-pkgs-dockerTools-buildImage
[go-reference-badge]: https://pkg.go.dev/badge/github.com/pdtpartners/nix-snapshotter.svg
[go-reference]: https://pkg.go.dev/github.com/pdtpartners/nix-snapshotter
[go-report-card-badge]: https://goreportcard.com/badge/github.com/pdtpartners/nix-snapshotter
[go-report-card]: https://goreportcard.com/report/github.com/pdtpartners/nix-snapshotter
[home-manager]: https://github.com/nix-community/home-manager
[manual-install]: docs/manual-install.md
[nix-command]: https://zero-to-nix.com/concepts/nix#unified-cli
[nixery]: https://nixery.dev/
[nix-flake]: https://zero-to-nix.com/concepts/flakes
[nix]: https://nixos.org/
[nix-installer]: https://zero-to-nix.com/start/install
[nixos]: https://zero-to-nix.com/concepts/nixos
