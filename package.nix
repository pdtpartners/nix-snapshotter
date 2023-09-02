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

      copyToRootFile =
        writeText
          "copy-to-root-${baseNameOf name}.json"
          (builtins.toJSON copyToRootList);

      fromImageFlag = lib.optionalString (fromImage != "") "--from-image ${fromImage}";

      image =
        let
          imageName = lib.toLower name;

          imageTag =
            if tag != null then tag
            else builtins.head (lib.strings.splitString "-" (baseNameOf image.outPath));
        in runCommand "nix-image-${baseNameOf name}.tar" {
          passthru = {
            inherit imageName;
            inherit imageTag;
            copyToRegistry = copyToRegistry image;
          };
        } ''
          ${nix-snapshotter}/bin/nix2container build \
            --config "${configFile}" \
            --closure "${runtimeClosureInfo}/store-paths" \
            --copy-to-root "${copyToRootFile}" \
            ${fromImageFlag} \
            ${imageName}:${imageTag} \
            $out
        '';

    in image;

  copyToRegistry = image: {
    imageName ? image.imageName,
    imageTag ? image.imageTag,
    plainHTTP ? false,
  }:
    let
      plainHTTPFlag = if plainHTTP then "--plain-http" else "";

    in writeShellScriptBin "copy-to-registry" ''
      ${nix-snapshotter}/bin/nix2container push \
        --ref "${imageName}:${imageTag}" \
        ${plainHTTPFlag} \
        ${image}
    '';

in nix-snapshotter
