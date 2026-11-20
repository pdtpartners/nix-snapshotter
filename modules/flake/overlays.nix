{ self, inputs, ... }:
{
  # Provide overlay to add `nix-snapshotter`.
  flake.overlays.default = self: super: {
    containerd-1_7 = super.containerd.overrideAttrs (_: rec {
      version = "1.7.28";

      src = self.fetchFromGitHub {
        owner = "containerd";
        repo = "containerd";
        tag = "v${version}";
        hash = "sha256-vz7RFJkFkMk2gp7bIMx1kbkDFUMS9s0iH0VoyD9A21s=";
      };

      outputs = [ "out" ];

      buildPhase = ''
        runHook preBuild
        patchShebangs .
        make binaries "VERSION=v${version}" "REVISION=${src.rev}"
        runHook postBuild
      '';

      installPhase = ''
        runHook preInstall
        install -Dm555 bin/* -t $out/bin
        runHook postInstall
      '';
    });

    # containerd 2.3's withReadonlyFS (oci.WithUser, WithAdditionalGIDs)
    # views the last mount as the rootfs, which for nix-snapshotter is a
    # store path bind mount: a single-file path fails with ENOTDIR and a
    # directory is silently read as /. Drop once containerd/containerd#14199
    # is in a release. The daemon's CRI uses it, nerdctl vendors it.
    containerd = super.containerd.overrideAttrs (old: {
      patches = (old.patches or []) ++ [ ./patches/containerd-fsview-child-target.patch ];
    });

    nerdctl = super.nerdctl.overrideAttrs (old: {
      # vendor/ is copied in (read-only) at configurePhase; see k3s below.
      postConfigure = (old.postConfigure or "") + ''
        fsview=vendor/github.com/containerd/containerd/v2/internal/fsview
        chmod u+w $fsview/mount.go $fsview
        patch --forward -p1 -d vendor/github.com/containerd/containerd/v2 \
          < ${./patches/containerd-fsview-child-target.patch}
      '';
    });

    nix-snapshotter = self.callPackage ../../package.nix {
      inherit (inputs) globset;
    };

    # Sandbox (pause) image as a nix:0 image. containerd fetches its sandbox
    # image itself, not through nix-snapshotter's image service, so the
    # modules preload it (see modules/nixos/containerd.nix).
    nix-snapshotter-pause = self.nix-snapshotter.buildImage {
      name = "pause";
      resolvedByNix = true;
      config.entrypoint = [ "${self.kubernetes.pause}/bin/pause" ];
    };

    # k3s 1.35 ships nix-snapshotter support (the snapshotter plugin and
    # the containerd config templates) itself, so its source is unpatched.
    k3s = super.k3s_1_35.override {
      buildGoModule = args:
        let
          isFunc = builtins.isFunction args;
          isK3s = !isFunc &&
                  args.pname != "k3s-cni-plugins" &&
                  args.pname != "k3s-containerd";

          checkpointFile = "vendor/github.com/containerd/containerd/v2/internal/cri/server/container_checkpoint_linux.go";
        in
          if isK3s then
            super.buildGoModule (args // {
              # Patch vendored containerd: treat ErrNotFound in checkpoint
              # detection as "not a checkpoint image" instead of a hard error.
              # This fixes a race where the CRI image store hasn't been
              # populated yet when CreateContainer runs.
              #
              # vendor/ doesn't exist at patchPhase (configurePhase copies it,
              # read-only, from goModules), so this runs in postConfigure:
              # unlike preBuild, goModules doesn't inherit it, and the vendor
              # hash stays upstream's. No `|| true`: a patch that stops
              # applying (or is already upstream) fails the build.
              postConfigure = (args.postConfigure or "") + ''
                chmod u+w ${checkpointFile} "$(dirname ${checkpointFile})"
                patch --forward -p1 < ${./patches/containerd-checkpoint-not-found.patch}
              '';
            })
          else
            super.buildGoModule args;
    };
  };

  perSystem =
    { system, ... }:
    {
      _module.args.pkgs = import inputs.nixpkgs {
        inherit system;
        # Apply default overlay to provide nix-snapshotter for NixOS tests &
        # configurations.
        overlays = [ self.overlays.default ];
      };
    };
}
