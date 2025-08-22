{ pkgs, lib, ... }@args:

import ./snapshotter.nix (args // {
  common = {
    nixpkgs.pkgs = lib.mkForce (pkgs.extend(self: super: {
      containerd = self.containerd-1_7;
    }));
  };
})
