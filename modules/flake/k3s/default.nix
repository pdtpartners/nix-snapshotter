{ lib, callPackage, ... }@args:

let
  k3s_builder = import ./builder.nix lib;
  common = opts: callPackage (k3s_builder opts);
  # extraArgs is the extra arguments passed in by the caller to propogate downward.
  # This is to allow all-packages.nix to do:
  #
  #     let k3s_1_23 = (callPackage ./path/to/k3s {
  #       commonK3sArg = ....
  #     }).k3s_1_23;
  extraArgs = builtins.removeAttrs args [ "callPackage" ];
in
{
  k3s_1_27 = common ((import ./1_27/versions.nix) // {
    updateScript = [ ./update-script.sh "27" ];
  }) extraArgs;
}
