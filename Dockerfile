ARG FLAKE_REF=github:pdtpartners/nix-snapshotter
FROM nixpkgs/nix-flakes@sha256:19accaca3dca5e3d6efe8da97c3d93ee9a1b2503702334d455e08eb07fc24dd1 AS nix

FROM nix AS base
ARG FLAKE_REF
RUN nix build "$FLAKE_REF#k3s"
RUN nix build "$FLAKE_REF#containerd"
RUN nix build "$FLAKE_REF#nix-snapshotter"

FROM base AS vm
ARG FLAKE_REF
RUN nix build \
	--out-link /vm \
	"$FLAKE_REF#nixosConfigurations.vm.config.system.build.vm"
RUN nix build \
	--out-link /vm-rootless \
	"$FLAKE_REF#nixosConfigurations.vm-rootless.config.system.build.vm"

FROM vm AS rootful
ENTRYPOINT [ "/vm/bin/run-nixos-vm" ]

FROM vm AS rootless
ENTRYPOINT [ "/vm-rootless/bin/run-nixos-vm" ]
