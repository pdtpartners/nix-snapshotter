# Contributing

Contributions should be made via pull requests. Pull requests will be reviewed
by one or more maintainers and merged when acceptable.

Pull requests will require NixOS tests to pass to merge, but will not run until
someone with triager or higher role adds the `ok-to-test` label.

Consider upstreaming anything that is not specific to nix-snapshotter to reduce
the amount that needs to be maintained in this repository.

In addition to the development tips below, do read through the
[Architecture](docs/architecture.md) docs to understand the big picture.

## Developing locally

When working on Golang changes in nix-snapshotter or nix2container, it is
usually faster compiling locally rather than inside of Nix. Any new features
should include accompanying Golang unit tests.

This project uses [direnv](https://github.com/direnv/direnv) to enter a
development environment. Otherwise, you can use `nix develop` as a replacement.

```sh
git clone https://github.com/pdtpartners/nix-snapshotter.git
cd nix-snapshotter
direnv allow # or `nix develop`
```

There is a `Makefile` for testing locally via rootless mode. This lets us run
containerd without root, but may need some setup on your machine.

> **Note**
> See [Rootless Containers][rootless-containers] for the prerequisites.

Start three terminals inside the development environment. In the first one,
run `make start-containerd` to start rootless containerd, which will be
configured to use nix-snapshotter.

```sh
make start-containerd
```

Then in second terminal, run `make start-nix-snapshotter`, a containerd proxy
plugin that provides a snapshotter that natively understands nix store paths
as well as regular image layers.

```sh
make start-nix-snapshotter
```

In a final terminal, run `make run-hello` will run a pre-built image from
`ghcr.io/pdtpartners/hello` with packages `pkgs.hello` from nixpkgs.

```sh
make run-hello
```

You can also try `make run-redis` which runs `pkgs.redis` from nixpkgs, which
also publishes ports to `:6379`, so you can try connecting to it via
`redis-cli`.

```sh
make run-redis
```

> **Note**
> Everything should be scoped to the git ignored `./build` directory, so a
> simple `make clean` should reset any persistent state.

If you want to inspect the [rootlesskit][rootlesskit] namespace, there is a `nsenter`
Makefile target to spawn a shell inside:

```sh
make nsenter ARGS="sh"
```

When modifying `pkg/nix2container`, like changing how the image manifest is
generated, you can build the image and untar it to a temporary directory to
poke around.

```sh
nix build ".#image-hello"
mkdir tmp
cd tmp
tar -xvf ../result
```

## Developing with NixOS VM

When working on the NixOS / Home Manager modules, or changes related to
Kubernetes, it's much easier to iterate inside the NixOS VM. New module options
or functionality across multiple services (nix-snapshotter, containerd,
Kubernetes) should include integration tests via NixOS tests under
`./modules/nixos/tests`.

Feel free to modify the `./modules/nixos/vm.nix` to include additional NixOS
configuration to aid testing.

```sh
nix run ".#vm"
```

Inside the VM, systemd commands will be helpful to diagnose issues, or
otherwise inspect/extract the `ExecStart` to isolate the problem outside of
systemd.

```sh
# When logged in as `root`.
journalctl -u containerd
journalctl -u nix-snapshotter
systemctl status kubernetes.slice

# When logged in as `rootless`.
journalctl --user -u containerd
journalctl --user -u nix-snapshotter
systemctl --user show nix-snapshotter
```

Instead of manually checking each time, consider running the NixOS tests
yourself to smoke test your changes.

```sh
nix run ".#test-snapshotter"
```

When working on the NixOS tests, the flake app `test-snapshotter` runs the test
driver directly on your host. This allows you to use the handy
`shell_interact()` method on the [machine object][machine-object] inside the
NixOS `testScript` to spawn a shell at that step.

## Implementation notes
- Despite not needing actual layer contents, each layer must have a non-zero
  size and valid digest on the registry side.
- Can get past upload requirements using "Non-Distributable" images.
  See: https://github.com/opencontainers/image-spec/blob/main/layer.md#non-distributable-layers
- Non-distributable images have media type `application/vnd.oci.image.layer.nondistributable.*`
- However, on registry side, non-distributable layers only allowed if
  descriptor "URLs" is also non-zero. The URL must be http or https scheme and
  without a fragment. And it must also pass an "allow" and "deny" regex which is
  all denied by registry defaults. It is unclear what is allowed on the GitHub Container Registry,
  and what is allowed on Artifactory registries.
- So we don't end up using non-distributable images.
- There are also "foreign" rootfs layers `application/vnd.docker.image.rootfs.foreign.*`,
  but this isn't supported by the distribution/distribution reference implementation.
  It is supported by containerd, but we need to upload to a registry so its a
  non-starter.
- We end up just using a regular OCI layer, with annotations.
- For remote snapshots, it need at least one layer that has
  `application/vnd.oci.image.layer.*` prefix for unpacker to unpack remotely.

[rootless-containers]: https://rootlesscontaine.rs/getting-started/common/
[rootlesskit]: https://github.com/rootless-containers/rootlesskit
[machine-object]: https://nixos.org/manual/nixos/stable/#ssec-machine-objects
