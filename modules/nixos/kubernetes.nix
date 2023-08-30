{ lib, config, pkgs, ... }:
{
  # Smooths out upstream service startup issues.
  imports = [ ./kubernetes-startup.nix ];

  # Provision single node kubernetes listening on localhost.
  services.kubernetes = {
    roles = ["master" "node"];
    masterAddress = "localhost";
  };

  # Do not take over cni/net.d as nerdctl wants it writeable as well.
  environment.etc = lib.mkMerge [
    { "cni/net.d".enable = false; }
    (
      lib.listToAttrs
        (lib.imap
          (i: entry:
            let name = "cni/net.d/${toString (10+i)}-${entry.type}.conf";
            in {
              inherit name;
              value = { source = pkgs.writeText name (builtins.toJSON entry); };
          })
          config.services.kubernetes.kubelet.cni.config
        )
    )
  ];

  # Allow non-root "admin" user to just use `kubectl`.
  services.certmgr.specs.clusterAdmin.private_key.owner = "rootless";
  environment.sessionVariables = {
    KUBECONFIG = "/etc/${config.services.kubernetes.pki.etcClusterAdminKubeconfig}";
  };
}
