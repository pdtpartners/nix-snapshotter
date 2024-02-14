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
    version = "0.1.1";
    src = lib.cleanSourceWith {
      src = lib.sourceFilesBySuffices ./. [
        ".go"
        "go.mod"
        "go.sum"
        ".tar"
      ];
    };
    vendorHash = "sha256-QBLePOnfsr6I19ddyZNSFDih6mCaZ/NV2Qz1B1pSHxs=";
    passthru = { inherit buildImage; };
  };

  # buildImage is analogous to the `docker build` command, in that it can be
  # used to build an OCI image archive that can be loaded into containerd. Note
  # Note that nix-snapshotter is a containerd plugin, so nix-snapshotter images
  # will only work with containerd.
  buildImage = args@{
    # The image name when exported. When resolvedByNix is enabled, this is
    # treated as just the package name to help identify the nix store path.
    name,
    # The image tag when exported. By default, this is the hash of the
    # derivation outPath.
    tag ? null,
    # If enabled, the OCI archive will be generated with a special image
    # reference in the format of "nix:0/nix/store/*.tar", which is resolvable
    # by nix-snapshotter if configured as the CRI image-service without a
    # Docker Registry.
    resolvedByNix ? false,
    # An image that is used as base image of this image. Any image can be used
    # as a fromImage, including non-nix images and images built with
    # pkgs.dockerTools.buildImage.
    fromImage ? null,
    # A derivation (or list of derivation) to include in the layer
    # root. The store path prefix /nix/store/hash-path is removed. The
    # store path content is then located at the image /.
    copyToRoot ? null,
    # An attribute set describing an image configuration as defined in:
    # https://github.com/opencontainers/image-spec/blob/8b9d41f48198a7d6d0a5c1a12dc2d1f7f47fc97f/specs-go/v1/config.go#L23
    config ? {},
  }:
    let
      baseName = baseNameOf name;

      configFile = writeText "config-${baseName}.json" (builtins.toJSON config);

      copyToRootList = lib.toList (args.copyToRoot or []);

      runtimeClosureInfo = closureInfo {
        rootPaths = [ configFile ] ++ copyToRootList;
      };

      copyToRootFile =
        writeText
          "copy-to-root-${baseName}.json"
          (builtins.toJSON copyToRootList);

      fromImageFlag = lib.optionalString (fromImage != null) ''--from-image "${fromImage}"'';

      image =
        let
          imageName = lib.toLower name;

          imageTag =
            if tag != null then tag
            else builtins.head (lib.strings.splitString "-" (baseNameOf image.outPath));

          imageRef = if resolvedByNix then "nix:0${image.outPath}" else "${imageName}:${imageTag}";

          refFlag = lib.optionalString (!resolvedByNix) ''--ref "${imageRef}"'';

        in runCommand "nix-image-${baseName}.tar" {
          passthru = {
            inherit name;
            tag = imageTag;
            # For kubernetes pod spec.
            image = imageRef;
            copyToRegistry = copyToRegistry image;
            copyToContainerd = copyToContainerd image;
          };
        } ''
          ${nix-snapshotter}/bin/nix2container build \
            --config "${configFile}" \
            --closure "${runtimeClosureInfo}/store-paths" \
            --copy-to-root "${copyToRootFile}" \
            ${refFlag} \
            ${fromImageFlag} \
            $out
        '';

    in image;

  # Copies an OCI archive to an OCI registry.
  copyToRegistry = image: {
    imageName ? image.name,
    imageTag ? image.tag,
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

  # Copies an OCI archive into containerd's image store.
  copyToContainerd = image: args@{
    address ? null,
    namespace ? null,
  }:
    let
      addressFlag =
        if args?address then "--address ${address}" else "";

      namespaceFlag =
        if args?namespace then "--namespace ${namespace}" else "";

    in writeShellScriptBin "copy-to-containerd" ''
      ${nix-snapshotter}/bin/nix2container \
        ${addressFlag} \
        ${namespaceFlag} \
        load ${image}
    '';

in nix-snapshotter
