[![ci workflow](https://github.com/pdtpartners/nix-snapshotter/actions/workflows/ci.yml/badge.svg)](https://github.com/pdtpartners/nix-snapshotter/actions?query=workflow%3ACI)
# nix-snapshotter

nix-snapshotter is a containerd snapshotter that understands nix store paths natively.

https://user-images.githubusercontent.com/6493975/208307213-705abd5c-b345-4b0c-b6ae-f64cce6edfef.mov

## Status

- nix-snapshotter works end to end with any arbitrary nix closure
- nix library similar to [nix2container](https://github.com/nlewo/nix2container) for building and pushing special nix container images
- creating snapshots sets up gcroots for nix images, containerd GCs gcroots, nix GCs store paths
- nix-snapshotter working inside kubernetes end to end (via [kind](https://kind.sigs.k8s.io/))

This package also still needs rigorous documentation, unit and integration testing.

## Running NixOS VM

The easiest way to try this out is to run a NixOS VM with containerd and
nix-snapshotter pre-configured. Run `nix run .#vm` to launch a graphic-less
NixOS VM that you can play around with immediately.

```sh
nix run ".#vm"
nixos login: root (Ctrl-A then X to quit)
Password: root

# Running pkgs.hello image with nix-snapshotter
nerdctl run ghcr.io/pdtpartners/hello

# Running pkgs.redis image with kubernetes & nix-snapshotter
kubectl apply -f /etc/kubernetes/redis/

# Wait a few seconds and a kubernetes service will be ready to forward port
# 30000 to the redis pod, so you can test it out with a `ping` command.
redis-cli -p 30000 ping
```

Or you can try running in rootless mode:

```sh
nix run ".#vm"
nixos login: rootless (Ctrl-A then X to quit)
Password: rootless

# Running pkgs.hello image with nix-snapshotter
nerdctl run ghcr.io/pdtpartners/hello

# Rootless kubernetes not supported yet
```

## Running locally

There is a `Makefile` for testing locally via rootless mode. This lets us run
containerd without root, but may need some setup on your machine.

See https://rootlesscontaine.rs/getting-started/common/ for the prerequisites.

This project also uses [direnv](https://github.com/direnv/direnv) to enter a
development environment. Otherwise, you can use `nix develop` as a replacement.

```sh
git clone https://github.com/pdtpartners/nix-snapshotter.git
cd nix-snapshotter
direnv allow # or `nix develop`
```

Start three terminals inside the development environment. In the first one,
run `make start-containerd` to start rootless containerd, which will be
configured to use nix-snapshotter.

```sh
$ make start-containerd
```

Then in second terminal, run `make start-nix-snapshotter`, a containerd proxy
plugin that provides a snapshotter that natively understands nix store paths
as well as regular image layers.

```sh
$ make start-nix-snapshotter
```

In a final terminal, run `make run-hello` will run a pre-built image from
the GitHub Container Registry `ghcr.io/pdtpartners/hello` with packages `pkgs.hello` from nixpkgs.

```sh
$ make run-hello
Hello, world!
```

You can also try `make run-redis` which runs `pkgs.redis` from nixpkgs, which
also publishes ports to `:6379`, so you can try connecting to it via
`redis-cli`.

```sh
$ make run-redis
1:C 18 Dec 2022 22:46:12.876 # oO0OoO0OoO0Oo Redis is starting oO0OoO0OoO0Oo
1:C 18 Dec 2022 22:46:12.876 # Redis version=7.0.5, bits=64, commit=00000000, modified=0, pid=1, just started
1:C 18 Dec 2022 22:46:12.876 # Warning: no config file specified, using the default config. In order to specify a config file use /nix/store/73cbgwvajchl067nv1jx43i65xxablri-redis-7.0.5/bin/redis-server /path/to/redis.conf
1:M 18 Dec 2022 22:46:12.876 # You requested maxclients of 10000 requiring at least 10032 max file descriptors.
1:M 18 Dec 2022 22:46:12.876 # Server can't set maximum open files to 10032 because of OS error: Operation not permitted.
1:M 18 Dec 2022 22:46:12.876 # Current maximum open files is 1024. maxclients has been reduced to 992 to compensate for low ulimit. If you need higher maxclients increase 'ulimit -n'.
1:M 18 Dec 2022 22:46:12.876 * monotonic clock: POSIX clock_gettime
1:M 18 Dec 2022 22:46:12.876 * Running mode=standalone, port=6379.
1:M 18 Dec 2022 22:46:12.876 # Server initialized
1:M 18 Dec 2022 22:46:12.876 # WARNING overcommit_memory is set to 0! Background save may fail under low memory condition. To fix this issue add 'vm.overcommit_memory = 1' to /etc/sysctl.conf and then reboot or run the command 'sysctl vm.overcommit_memory=1' for this to take effect.
1:M 18 Dec 2022 22:46:12.877 * Ready to accept connections
```

## Example image

There is an example container image for use with this snapshotter pushed to
the GitHub Container Registry as `ghcr.io/pdtpartners/hello`:

```
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "digest": "sha256:fc237fee0c406884552ec8202fdcbd1350829ccdc5b47951f59e2e1c75d734d1",
    "size": 311
  },
  "layers": [
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
      "digest": "sha256:126ab0b174a8f4dcdcde9b6a2675ecee0ab107127d0a96fe885938128d2884da",
      "size": 343,
      "annotations": {
        "containerd.io/snapshot/nix-layer": "true",
        "containerd.io/snapshot/nix-store-path.0": "/nix/store/3n58xw4373jp0ljirf06d8077j15pc4j-glibc-2.37-8",
        "containerd.io/snapshot/nix-store-path.1": "/nix/store/fz2c8qahxza5ygy4yvwdqzbck1bs3qag-libidn2-2.3.4",
        "containerd.io/snapshot/nix-store-path.2": "/nix/store/q7hi3rvpfgc232qkdq2dacmvkmsrnldg-libunistring-1.1",
        "containerd.io/snapshot/nix-store-path.3": "/nix/store/ryvnrp5n6kqv3fl20qy2xgcgdsza7i0m-xgcc-12.3.0-libgcc",
        "containerd.io/snapshot/nix-store-path.4": "/nix/store/s66mzxpvicwk07gjbjfw9izjfa797vsw-hello-2.12.1"
      }
    }
  ]
}
```

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
