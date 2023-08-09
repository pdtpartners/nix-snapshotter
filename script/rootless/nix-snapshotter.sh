(cd ../../ && make nix-snapshotter)
mkdir -p /home/buxton/.local/share/containerd/
mkdir -p /run/user/1001/containerd-nix/
../../out/nix-snapshotter /run/user/1001/containerd-nix/containerd-nix.sock /home/buxton/.local/share/containerd