ARG FLAKE_REF=github:pdtpartners/nix-snapshotter
FROM nixpkgs/nix-flakes AS nix

FROM nix AS base
RUN nix build "$FLAKE_REF#k3s"
RUN nix build "$FLAKE_REF#containerd"
RUN nix build "$FLAKE_REF#nix-snapshotter"

FROM base AS vm
RUN nix build \
	--out-link /vm \
	"#nixosConfigurations.vm.config.system.build.vm"
RUN nix build \
	--out-link /vm-rootless \
	"$FLAKE_REF#nixosConfigurations.vm-rootless.config.system.build.vm"

FROM vm AS rootful
ENTRYPOINT [ "/vm/bin/run-nixos-vm" ]

FROM vm AS rootless
ENTRYPOINT [ "/vm-rootless/bin/run-nixos-vm" ]
