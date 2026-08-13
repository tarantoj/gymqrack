# NixOS module to run the VivaGym Access live-QR server as a systemd service.
#
# Example usage:
#
#   {
#     inputs.vivagym-access.url = "github:you/vivagym-access";
#     ...
#   }
#
#   services.vivagym-access = {
#     enable = true;
#     publicUrl = "https://qr.example.com";
#     trustProxy = true;
#     clientId = "your-client-id";
#     clientSecret = "your-client-secret";
#     # or load VIVAGYM_CLIENT_ID / VIVAGYM_CLIENT_SECRET from a root-owned file:
#     # environmentFile = "/run/secrets/vivagym-access.env";
#   };
#
# Users authenticate through the web UI (email + password); the server is a
# stateless proxy and never stores their credentials. Put it behind a reverse
# proxy (nginx/caddy) to expose it over HTTPS.

{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.services.vivagym-access;
in
{
  options.services.vivagym-access = {
    enable = lib.mkEnableOption "the VivaGym Access live-QR server";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ../package.nix { };
      defaultText = lib.literalExpression "pkgs.callPackage ../package.nix { }";
      description = "Package providing the vivagym-access server.";
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
      description = "Public base URL of the live-QR page (served to the web UI).";
    };

    locale = lib.mkOption {
      type = lib.types.str;
      default = "es";
      description = "VivaGym API locale (es, en, pt).";
    };

    loginRatePerMinute = lib.mkOption {
      type = lib.types.int;
      default = 10;
      description = "Max login attempts per minute per IP.";
    };

    cookieMaxAgeDays = lib.mkOption {
      type = lib.types.int;
      default = 7;
      description = "Lifetime of the client-held session cookie (days).";
    };

    trustProxy = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Honor X-Forwarded-For for login rate limiting (set behind a reverse proxy).";
    };

    clientId = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "VivaGym OAuth client id (VIVAGYM_CLIENT_ID). Required unless provided via environmentFile.";
    };

    clientSecret = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "VivaGym OAuth client secret (VIVAGYM_CLIENT_SECRET). Required unless provided via environmentFile.";
    };

    environmentFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "File (e.g. /run/secrets/vivagym-access.env) holding VIVAGYM_CLIENT_ID and VIVAGYM_CLIENT_SECRET, loaded by systemd. Alternative to setting clientId/clientSecret as options.";
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = (cfg.clientId != null) == (cfg.clientSecret != null);
        message = "services.vivagym-access: set both clientId and clientSecret, or neither (use environmentFile instead).";
      }
      {
        assertion = cfg.clientId != null || cfg.environmentFile != null;
        message = "services.vivagym-access: clientId and clientSecret must be set, or environmentFile must point to a file containing VIVAGYM_CLIENT_ID and VIVAGYM_CLIENT_SECRET.";
      }
    ];

    systemd.services.vivagym-access = {
      description = "VivaGym Access live-QR server";
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      wantedBy = [ "multi-user.target" ];

      serviceConfig = {
        Type = "simple";
        ExecStart = "${cfg.package}/bin/vivagym-access";
        DynamicUser = true;
        Restart = "on-failure";
        RestartSec = 5;
        EnvironmentFile = lib.mkIf (cfg.environmentFile != null) cfg.environmentFile;
        Environment = [
          "VIVAGYM_LOCALE=${cfg.locale}"
          "PORT=${toString cfg.port}"
          "HOST=${cfg.host}"
          "PUBLIC_URL=${cfg.publicUrl}"
          "LOGIN_RATE_PER_MIN=${toString cfg.loginRatePerMinute}"
          "COOKIE_MAX_AGE_DAYS=${toString cfg.cookieMaxAgeDays}"
          "TRUST_PROXY=${if cfg.trustProxy then "1" else "0"}"
        ]
        ++ lib.optional (cfg.clientId != null) "VIVAGYM_CLIENT_ID=${cfg.clientId}"
        ++ lib.optional (cfg.clientSecret != null) "VIVAGYM_CLIENT_SECRET=${cfg.clientSecret}";
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectHome = true;
        ProtectSystem = "strict";
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        RestrictAddressFamilies = [
          "AF_INET"
          "AF_INET6"
        ];
      };
    };
  };
}
