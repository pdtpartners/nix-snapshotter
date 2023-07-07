{ pkgs ? import <nixpkgs> { }, system }:
let
  l = pkgs.lib // builtins;

  nix-snapshotter = pkgs.buildGoModule {
    pname = "nix-snapshotter";
    version = "0.0.1";
    src = l.cleanSourceWith {
      src = ./.;
      filter = path: type:
      let
        p = baseNameOf path;
      in !(
        p == "flake.nix" ||
        p == "flake.lock" ||
        p == "README.md" ||
        p == "default.nix"
      );
    };
    vendorSha256 = "sha256-eFfzuHLcHRru4jNkllY9HpqNkEwcBzaKmoaZaum8KvU=";
  };

  buildImage = args@{
    # The image name when exported.
    name,
    # The image tag when exported.
    tag ? null,
    # An image that is used as base image of this image.
    fromImage ? "",
    # A derivation (or list of derivation) to include in the layer
    # root. The store path prefix /nix/store/hash-path is removed. The
    # store path content is then located at the image /.
    copyToRoot ? null,
    # An attribute set describing an image configuration as defined in:
    # https://github.com/opencontainers/image-spec/blob/8b9d41f48198a7d6d0a5c1a12dc2d1f7f47fc97f/specs-go/v1/config.go#L23
    config ? {},
  }:
    let
      configFile = pkgs.writeText "config-${baseNameOf name}.json" (l.toJSON config);
      copyToRootList = l.toList (args.copyToRoot or []);
      closureInfo = pkgs.closureInfo {
        rootPaths = [ configFile ] ++ copyToRootList;
      };
      copyToRootFile = pkgs.writeText "copy-to-root-${baseNameOf name}.json" (l.toJSON copyToRootList);
      fromImageFlag = l.optionalString (fromImage != "") "--from-image ${fromImage}";
      image = let
        imageName = l.toLower name;
        imageTag =
          if tag != null
          then tag
          else
          l.head (l.strings.splitString "-" (baseNameOf image.outPath));
      in pkgs.runCommand "image-${baseNameOf name}.json"
      {
        inherit imageName;
        passthru = {
          inherit imageTag;
          # provide a cheap to evaluate image reference for use with external tools like docker
          # DO NOT use as an input to other derivations, as there is no guarantee that the image
          # reference will exist in the store.
          imageRefUnsafe = l.unsafeDiscardStringContext "${imageName}:${imageTag}";
          copyToRegistry = copyToRegistry image;
        };
      }
      ''
        ${nix-snapshotter}/bin/nix2container build \
        ${fromImageFlag} \
        ${configFile} \
        ${closureInfo}/store-paths \
        ${copyToRootFile} \
        $out
      '';
    in image;

  copyToRegistry = image: pkgs.writeShellScriptBin "copy-to-registry" ''
    echo "Copy to Docker registry image ${image.imageName}:${image.imageTag}"
    ${nix-snapshotter}/bin/nix2container push \
    ${image} \
    ${image.imageName}:${image.imageTag}
  '';

in
{
  inherit nix-snapshotter buildImage;
}
