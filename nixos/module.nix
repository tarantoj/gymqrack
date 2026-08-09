# NixOS module to run the VivaGym Wallet live-QR server as a systemd service.
#
# Example usage:
#
#   {
#     inputs.vivagym-wallet.url = "github:you/vivagym-wallet";
#     ...
#   }
#
#   services.vivagym-wallet = {
#     enable = true;
#     publicUrl = "https://qr.example.com";
#     email = "member@example.com";
#     passwordFile = config.age.secrets."vivagym-password".path; # e.g. sops-nix / agenix
#   };
#
# The server listens on host:port (127.0.0.1:4567 by default); put it behind
# a reverse proxy (nginx/caddy) to expose it over HTTPS.

{ config, lib, pkgs, ... }:

let
  cfg = config.services.vivagym-wallet;
in
{
  options.services.vivagym-wallet = {
    enable = lib.mkEnableOption "the VivaGym Wallet live-QR server";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ../package.nix { };
      defaultText = lib.literalExpression "pkgs.callPackage ../package.nix { }";
      description = "Package providing the vivagym-wallet server.";
    };

    host = lib.mkOption {
      type = lib.types.str;
      default = "127.0.0.1";
      description = "Address to bind. Expose over a reverse proxy, not publicly.";
    };

    port = lib.mkOption {
      type = lib.types.port;
      default = 4567;
      description = "Port to listen on.";
    };

    publicUrl = lib.mkOption {
      type = lib.types.str;
      default = "http://localhost:4567";
      description = "Public base URL of the live-QR page (served to the wallet pass / UI).";
    };

    locale = lib.mkOption {
      type = lib.types.str;
      default = "es";
      description = "VivaGym API locale (es, en, pt).";
    };

    email = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "VivaGym member email (username). Can also be set in passwordFile.";
    };

    passwordFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = ''
        Path to a file containing the member password, e.g. `VIVAGYM_PASSWORD=...`.
        Point this at a secret from sops-nix/agenix so the password never lands in
        the Nix store. VIVAGYM_EMAIL may also be set here.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    systemd.services.vivagym-wallet = {
      description = "VivaGym Wallet live-QR server";
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      wantedBy = [ "multi-user.target" ];

      serviceConfig = {
        Type = "simple";
        ExecStart = "${cfg.package}/bin/vivagym-wallet";
        DynamicUser = true;
        Restart = "on-failure";
        RestartSec = 5;
        Environment = [
          "VIVAGYM_LOCALE=${cfg.locale}"
          "PORT=${toString cfg.port}"
          "HOST=${cfg.host}"
          "PUBLIC_URL=${cfg.publicUrl}"
        ] ++ lib.optionals (cfg.email != "") [ "VIVAGYM_EMAIL=${cfg.email}" ];
        EnvironmentFile = lib.mkIf (cfg.passwordFile != null) cfg.passwordFile;
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectHome = true;
        ProtectSystem = "strict";
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" ];
      };
    };
  };
}
