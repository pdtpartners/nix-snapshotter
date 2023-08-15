[![ci workflow](https://github.com/pdtpartners/nix-snapshotter/actions/workflows/ci.yml/badge.svg)](https://github.com/pdtpartners/nix-snapshotter/actions?query=workflow%3ACI)
# nix-snapshotter

Containerd remote snapshotter that prepares container rootfs from nix store directly.

https://user-images.githubusercontent.com/6493975/208307213-705abd5c-b345-4b0c-b6ae-f64cce6edfef.mov

## Status

- nix-snapshotter works end to end with any arbitrary nix closure
- nix library similar to [nix2container](https://github.com/nlewo/nix2container) for building and pushing special nix container images
- creating snapshots sets up gcroots for nix images, containerd GCs gcroots, nix GCs store paths
- nix-snapshotter working inside kubernetes end to end (via [kind](https://kind.sigs.k8s.io/))

This package also still needs rigorous documentation, unit and integration testing.

## Example image

There is an example container image for use with this snapshotter pushed to DockerHub as `hinshun/hello:nix`:

```
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "digest": "sha256:d0b29db9c2d41192481511b7ed1aea271708290a4b74c0f7caf02be54c083d7b",
    "size": 311
  },
  "layers": [
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
      "digest": "sha256:fa013ec743e03c288f6a8ab8b15ebe1eb9f56dc290b668f16aee263fe29dd600",
      "size": 317,
      "annotations": {
        "containerd.io/snapshot/nix-layer": "true",
        "containerd.io/snapshot/nix/store.0": "34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
        "containerd.io/snapshot/nix/store.1": "4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
        "containerd.io/snapshot/nix/store.2": "5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
        "containerd.io/snapshot/nix/store.3": "g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1"
      }
    }
  ]
}
```

## Running

There is a `Makefile` for testing locally. Though it requires a development
environment where you have access to root.

If you have [direnv](https://github.com/direnv/direnv), run `direnv allow` to enter a development environment,
otherwise run `nix develop` in each of the terminals you'll have to manage.
Then, inside the development environment run `make start-containerd` to start
the container supervisor. This containerd will be configured to use proxy plugin
`nix` for the snapshotter.

```sh
$ make start-containerd
```

Then in another terminal, start `nix-snapshotter`, a GRPC service that
implements a containerd snapshotter.

```sh
$ make start-nix-snapshotter
```

In a final terminal, `make run` will use the CRI interface to pull the
prebuilt `hinshun/hello:nix` image and run it.

```sh
$ make run
Hello, world!
```

A more complicated example is `hinshun/redis:nix`:
```
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

## Implementation notes
- Despite not needing actual layer contents, each layer must have a non-zero
  size and valid digest on the registry side.
- Can get past upload requirements using "Non-Distributable" images.
  See: https://github.com/opencontainers/image-spec/blob/main/layer.md#non-distributable-layers
- Non-distributable images have media type `application/vnd.oci.image.layer.nondistributable.*`
- However, on registry side, non-distributable layers only allowed if
  descriptor "URLs" is also non-zero. The URL must be http or https scheme and
  without a fragment. And it must also pass an "allow" and "deny" regex which is
  all denied by registry defaults. It is unclear what is allowed on Docker Hub,
  and what is allowed on Artifactory registries.
- So we don't end up using non-distributable images.
- There are also "foreign" rootfs layers `application/vnd.docker.image.rootfs.foreign.*`,
  but this isn't supported by the distribution/distribution reference implementation.
  It is supported by containerd, but we need to upload to a registry so its a
  non-starter.
- We end up just using a regular OCI layer, with annotations.
- For remote snapshots, it need at least one layer that has
  `application/vnd.oci.image.layer.*` prefix for unpacker to unpack remotely.
