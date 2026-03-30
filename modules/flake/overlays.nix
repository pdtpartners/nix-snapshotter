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

    nix-snapshotter = self.callPackage ../../package.nix {
      inherit (inputs) globset;
    };

    k3s = super.k3s_1_34.override {
      buildGoModule = args:
        let
          isFunc = builtins.isFunction args;
          shouldPatch = !isFunc &&
                        args.pname != "k3s-cni-plugins" &&
                        args.pname != "k3s-containerd";

          patchedSrc = super.runCommand "k3s-patched-src" {} ''
            cp -r ${args.src} $out
            chmod -R u+w $out
            cd $out
            patch -p1 < ${./patches/k3s-nix-snapshotter.patch}
          '';
        in
          if shouldPatch then
            super.buildGoModule (args // {
              src = patchedSrc;
              vendorHash = {
                # k3s 1.34.2+k3s1
                "sha256-IJi5gVxBsAjeQHi5rQpNRvWOXuNPx2Rtsy18VL+2Yxo=" = "sha256-Y3Dc/aWNpiNxDaJb3RAudwN7Ep6WdhSCQmtjt1pNk1w=";
                # k3s 1.34.3+k3s3
                "sha256-R8QXwXmTKsONsbWaedFNDPdYZ82jaQ/T8S9sllqKPjk=" = "sha256-IaWUzoMAye85cNkjE5ISJkDujO6PsSsKy8l1CH7SimY=";
                # k3s 1.34.4+k3s1
                "sha256-ZTRcv28rgKslrDRr5y8SnQJpo2ErbURa22l1nv+4QHw=" = "sha256-OK79hUWRJ8MvvMyy0vts6Bu8gudEANHQ9YAemZfXrsw=";
                # k3s 1.34.5+k3s1
                "sha256-q3/KylcuuhUMC3ggpR8DsLjdWgtPnhCqa1HjM2sgHuo=" = "sha256-ybfcn5VAqhRd9CavOMBsfgzqhgOVIt/P2NOE/wgRn2k=";
              }.${args.vendorHash};
              # Patch vendored containerd: treat ErrNotFound in checkpoint
              # detection as "not a checkpoint image" instead of a hard error.
              # This fixes a race where the CRI image store hasn't been
              # populated yet when CreateContainer runs.
              preBuild = (args.preBuild or "") + ''
                patch --forward -p1 < ${./patches/containerd-checkpoint-not-found.patch} || true
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
