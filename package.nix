{ lib
, buildGoModule
, closureInfo
, runCommand
, writeShellScriptBin
, writeText
}:

let
  nix-snapshotter = buildGoModule {
    pname = "nix-snapshotter";
    version = "0.0.1";
    src = lib.cleanSourceWith {
      src = lib.sourceFilesBySuffices ./. [
        ".go"
        "go.mod"
        "go.sum"
        ".tar"
      ];
    };
    vendorSha256 = "sha256-l0ttbSToudTT+GloxOZE6ohGIx8/OTq2LFCi1rjk7Ec=";
    passthru = { inherit buildImage; };
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
      configFile = writeText "config-${baseNameOf name}.json" (builtins.toJSON config);
      copyToRootList = lib.toList (args.copyToRoot or []);
      runtimeClosureInfo = closureInfo {
        rootPaths = [ configFile ] ++ copyToRootList;
      };
      copyToRootFile = writeText "copy-to-root-${baseNameOf name}.json" (builtins.toJSON copyToRootList);
      fromImageFlag = lib.optionalString (fromImage != "") "--from-image ${fromImage}";
      image = let
        imageName = lib.toLower name;
        imageTag =
          if tag != null
          then tag
          else
          builtins.head (lib.strings.splitString "-" (baseNameOf image.outPath));
      in runCommand "image-${baseNameOf name}.json"
      {
        inherit imageName;
        passthru = {
          inherit imageTag;
          # provide a cheap to evaluate image reference for use with external tools like docker
          # DO NOT use as an input to other derivations, as there is no guarantee that the image
          # reference will exist in the store.
          imageRefUnsafe = builtins.unsafeDiscardStringContext "${imageName}:${imageTag}";
          copyToRegistry = copyToRegistry image;
          copyToOCIArchive = copyToOCIArchive image;
        };
      }
      ''
        ${nix-snapshotter}/bin/nix2container build \
        ${fromImageFlag} \
        ${configFile} \
        ${runtimeClosureInfo}/store-paths \
        ${copyToRootFile} \
        $out
      '';
    in image;

  copyToRegistry = image: {
    plainHTTP ? false
  }:
    let
      plainHTTPFlag = if plainHTTP then "--plain-http" else "";

    in writeShellScriptBin "copy-to-registry" ''
      echo "Copy ${image.imageName}:${image.imageTag} to Docker Registry"
      ${nix-snapshotter}/bin/nix2container push \
      ${plainHTTPFlag} \
      ${image} \
      ${image.imageName}:${image.imageTag}
    '';

  copyToOCIArchive = image: {}:
    runCommand "${baseNameOf image.imageName}.tar" {} ''
      echo "Copy ${image.imageName}:${image.imageTag} to OCI archive"
      ${nix-snapshotter}/bin/nix2container export \
      ${image} \
      ${image.imageName}:${image.imageTag} \
      $out
    '';

in nix-snapshotter
