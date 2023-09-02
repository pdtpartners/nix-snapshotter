{ lib }:
{
  home-manager = {
    # Converts a home-manager systemd user service to a NixOS systemd user
    # service. Since home-manager style services map closer to raw systemd
    # service specification, it's easier to transform in this direction.
    convertServiceToNixOS = unit:
      {
        serviceConfig = lib.optionalAttrs (unit?Service) unit.Service;
        unitConfig = lib.optionalAttrs (unit?Unit) unit.Unit;
      } // (lib.optionalAttrs (unit?Install.WantedBy) {
        # Only `WantedBy` is supported by NixOS as [Install] fields are not
        # supported, due to its stateful nature.
        wantedBy = unit.Install.WantedBy;
      });
  };
}
