{ name ? "redis" }:

let
  nixpkgs =
    (
      let lock = builtins.fromJSON (builtins.readFile ../flake.lock);
      in fetchTarball {
        url = "https://github.com/NixOS/nixpkgs/archive/${lock.nodes.nixpkgs.locked.rev}.tar.gz";
        sha256 = lock.nodes.nixpkgs.locked.narHash;
      }
    );

  nix-snapshotter = import ../.;

  config =
    if name == "redis" then
      {
        entrypoint = "redis-server";
        port = 6379;
        args = [ "--protected-mode" "no" ];
        nodePort = 30000;
      }
    else
      {
        entrypoint = "etcd";
        port = 2379;
        args = [
          "--listen-client-urls"
          "http://0.0.0.0:2379"
          "--advertise-client-urls"
          "http://0.0.0.0:2379"
        ];
        nodePort = 30001;
      };

  inherit (config)
    entrypoint
    port
    args
    nodePort
  ;

  pkgs = import nixpkgs {
    overlays = [ nix-snapshotter.overlays.default ];
  };

  image = pkgs.nix-snapshotter.buildImage {
    inherit name;
    resolvedByNix = true;
    config = {
      entrypoint = [ "${pkgs.${name}}/bin/${entrypoint}" ];
    };
  };

  pod = pkgs.writeText "${name}-pod.json" (builtins.toJSON {
    apiVersion = "v1";
    kind = "Pod";
    metadata = {
      inherit name;
      labels = { inherit name; };
    };
    spec.containers = [{
      inherit name args;
      image = "nix:0${image}";
      ports = [{
        name = "client";
        containerPort = port;
      }];
    }];
  });

  service = pkgs.writeText "${name}-service.json" (builtins.toJSON {
    apiVersion = "v1";
    kind = "Service";
    metadata.name = "${name}-service";
    spec = {
      type = "NodePort";
      selector = { inherit name; };
      ports = [{
        name = "client";
        inherit port nodePort;
      }];
    };
  });

in pkgs.runCommand "declarative-k8s" {} ''
  mkdir -p $out/share/k8s
  cp ${pod} $out/share/k8s/
  cp ${service} $out/share/k8s/
''
