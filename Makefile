PREFIX ?= $(CURDIR)/out/

CMD=nix-snapshotter

.PHONY: all build nix-snapshotter start-containerd start-nix-snapshotter run ctr-run-redis clean

all: build

build: $(CMD)

FORCE:

nix-snapshotter: FORCE
	go build -o $(PREFIX) .

start-containerd:
	sudo containerd --log-level debug --config ./script/config/etc/containerd/config.toml

start-nix-snapshotter: nix-snapshotter
	mkdir -p root
	sudo mkdir -p /run/containerd-nix
	sudo ./out/nix-snapshotter /run/containerd-nix/containerd-nix.sock $$(pwd)/root

run:
	sudo crictl pull docker.io/hinshun/hello:nix
	sudo ctr --namespace k8s.io run --rm --snapshotter nix docker.io/hinshun/hello:nix example

run-redis:
	sudo crictl pull docker.io/library/redis:alpine
	sudo ctr --namespace k8s.io run --snapshotter nix --rm docker.io/library/redis:alpine redis

clean:
	sudo rm -rf ./root
	sudo rm -rf /run/containerd
	sudo rm -rf /run/containerd-nix
	sudo rm -rf /var/lib/containerd
