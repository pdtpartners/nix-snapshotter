{ lib, ... }:
{
  perSystem = { pkgs, ... }:
    let
      linters = {
        golangci-lint = pkgs.writeScriptBin "golangci-lint" ''
          ${pkgs.golangci-lint}/bin/golangci-lint run -v
        '';
      };

      apps =
        lib.mapAttrs'
          (name: program:
            lib.nameValuePair
              ("lint-" + name)
              {
                type = "app";
                program = "${program}/bin/${name}";
              }
          )
          linters;

    in { inherit apps; };
}
