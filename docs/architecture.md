# Architecture

[Containerd][containerd] is an industry standard container runtime, used as the
default runtime behind Docker and Kubernetes. It's intended to be used as a
low-level daemon to be embedded in a bigger system. Containerd has a
[plugin architecture][plugin-architecture] that allows most of its subsystems
to be replaced by an external process communicating over gRPC.

nix-snapshotter is a [snapshotter][snapshotter] plugin for containerd. The
snapshotter is responsible for returning a list of mounts for the runtime to
then execute the syscalls to prepare the container rootfs (root filesystem).
Since containerd has first-class support for plugins, this means
nix-snapshotter works with off-the-shelf containerd. However, we do require
a pretty recent version because we depend on bug fixes that we upstreamed.

nix-snapshotter also leverages [remote snapshotter][remote-snapshotter]
features to take over the mechanism of unpacking layers during image pull. This
allows nix-snapshotter to look at the layer's annotations where we keep the Nix
store paths to create GC roots (substituting from a binary cache if necessary).
An unpacked layer is known as a `snapshot`, which allows branching if used as
a base image. Each Nix snapshot has a corresponding `gcroots` directory where
Nix out-links are created.

## Image manifest

Here is an example Nix image manifest pushed to `ghcr.io/pdtpartners/hello`:

```json
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

> **Note**
> This manifest respects the [OCI specification][oci-spec], so we're able to
> push it to any OCI-compliant registry.

## Garbage collection

Containerd and Nix both have their own reference-counting garbage collector.
All snapshotter implementations reference blobs in some way to avoid
collecting. nix-snapshotter instead reference nix store paths by the way of
Nix GC roots.

When containerd GC runs, deleted snapshots will remove the corresponding Nix
GC roots. When Nix GC runs, Nix store paths without references will be
deleted. As long as both have reasonable GC policies, Nix store paths
referenced by container images will be garbage collected as designed by both
systems.

## Mounts

Nix-snapshotter embeded the upstream overlay snapshotter in order to have full
support for regular layer-based images. Since overlayfs has a
[128 lowerdir limit][lowerdir-limit], we create a bind mount for every Nix
store path instead. There is no known limit of how many bind mounts you can
have simultaneously, we'll be testing nix-snapshotter with large Nix closures to
test the boundaries.

Regardless of whether there is a regular layer involved, nix-snapshotter will
create an overlayfs mount to provide a read-writable container rootfs. If the
container is created with a readonly rootfs, then the overlayfs mount is
omitted.

Since each mount requires a mountpoint to exist prior to the syscall, as part
of `pkgs.nix-snapshotter.buildImage`, we generate a tarball with an empty
directory for each Nix store path. Technically we can do this during unpack,
but since it's tiny, it is simpler to just build it and store it in the Nix
store.

## Implementation quirks

If we decide move the mountpoints tarball generation to unpack time, note that
zero size layers is undefined in the [OCI specification][oci-spec]. The
[Docker Registry][distribution] implementation doesn't support zero size layers
and must have a valid digest.

Alternatively, the layer tarball can be avoided if we use
[Non-distributable layers][nondistributable] images. However in the Docker
Registry implementation, non-distributable layers are only allowed if
descriptor "URLs" is also non-zero. The URL must be http or https scheme and
without a fragment. It must also pass an "allow" and "deny" regex which is
defaulted to deny all, which makes it a non-starter.

There is also `foreign` layers with media type
`application/vnd.docker.image.rootfs.foreign.*` but it has been superseded
by non-distributable layers.

[containerd]: https://github.com/containerd/containerd
[remote-snapshotter]: https://github.com/containerd/containerd/blob/v1.7.2/docs/remote-snapshotter.md
[plugin-architecture]: https://github.com/containerd/containerd/blob/v1.7.2/docs/PLUGINS.md
[snapshotter]: https://github.com/containerd/containerd/blob/v1.7.2/docs/snapshotters/README.md
[oci-spec]: https://github.com/opencontainers/image-spec
[lowerdir-limit]: https://github.com/moby/moby/issues/26380
[distribution]: https://github.com/distribution/distribution
[nondistributable]: https://github.com/opencontainers/image-spec/blob/v1.0.2/layer.md#non-distributable-layers
