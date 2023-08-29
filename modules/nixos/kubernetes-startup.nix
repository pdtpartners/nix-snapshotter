{ lib, config, pkgs, ... }:
let
  inherit (config.systemd.services)
    kube-addon-manager
  ;

  inherit (config.services)
    certmgr
  ;

  cfg = config.services.kubernetes;

  waitFile = filename: toString (pkgs.writeShellScript "wait-file-${filename}" ''
    while [ ! -f /var/lib/kubernetes/secrets/${filename} ]; do sleep 1; done
  '');

  waitPort = port: toString (pkgs.writeShellScript "wait-port-${port}" ''
    while ! ${pkgs.netcat}/bin/nc -z localhost ${port}; do sleep 1; done
  '');

in {
  # Fix various startup issues related to kubernetes systemd services to avoid
  # failures during NixOS VM boot.
  config = lib.mkMerge [
    (lib.mkIf (cfg.roles != []) {
      systemd.services.etcd.preStart = waitFile "etcd.pem";
      systemd.services.certmgr.preStart = lib.mkForce ''
        mkdir -p ${cfg.secretsPath}
        ${waitFile "ca.pem"}
      '';
      systemd.services.kube-apiserver = {
        preStart = waitFile "service-account-key.pem";
        # Wait for its own securePort to be ready since it doesn't support
        # systemd notify.
        postStart = waitPort (toString cfg.apiserver.securePort);
      };
    })
    (lib.mkIf (cfg.addonManager.bootstrapAddons != {}) {
      systemd.services.kube-addon-manager.preStart =
        let
          files = lib.mapAttrsToList (n: v: pkgs.writeText "${n}.json" (builtins.toJSON v))
            cfg.addonManager.bootstrapAddons;

        in lib.mkForce ''
          ${waitFile "cluster-admin.pem"}
          export KUBECONFIG="/etc/${cfg.pki.etcClusterAdminKubeconfig}"
          ${cfg.package}/bin/kubectl apply -f ${lib.concatStringsSep " \\\n -f " files}
        '';
    })
  ];
}
