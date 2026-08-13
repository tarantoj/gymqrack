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
#     trustProxy = true;
#     clientId = "your-client-id";
#     clientSecret = "your-client-secret";
#     # or load VIVAGYM_CLIENT_ID / VIVAGYM_CLIENT_SECRET from a root-owned file:
#     # environmentFile = "/run/secrets/vivagym-wallet.env";
#     # telemetry (optional): export OpenTelemetry traces to an OTLP collector,
#     # otherwise they go to stdout.
#     # telemetryOtlpEndpoint = "https://collector.example.com:4318";
#     # telemetryServiceName = "vivagym-wallet";
#   };
#
# Users authenticate through the web UI (email + password); the server is a
# stateless proxy and never stores their credentials. Put it behind a reverse
# proxy (nginx/caddy) to expose it over HTTPS.

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
      description = "File (e.g. /run/secrets/vivagym-wallet.env) holding VIVAGYM_CLIENT_ID and VIVAGYM_CLIENT_SECRET, loaded by systemd. Alternative to setting clientId/clientSecret as options.";
    };

    telemetryOtlpEndpoint = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "OTLP HTTP collector endpoint. When set, OpenTelemetry traces are exported to it; otherwise they are written to stdout. Exporter headers can be set via OTEL_EXPORTER_OTLP_HEADERS in environmentFile.";
    };

    telemetryServiceName = lib.mkOption {
      type = lib.types.str;
      default = "vivagym-wallet";
      description = "OpenTelemetry service name (OTEL_SERVICE_NAME).";
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = (cfg.clientId != null) == (cfg.clientSecret != null);
        message = "services.vivagym-wallet: set both clientId and clientSecret, or neither (use environmentFile instead).";
      }
      {
        assertion = cfg.clientId != null || cfg.environmentFile != null;
        message = "services.vivagym-wallet: clientId and clientSecret must be set, or environmentFile must point to a file containing VIVAGYM_CLIENT_ID and VIVAGYM_CLIENT_SECRET.";
      }
    ];

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
        EnvironmentFile = lib.mkIf (cfg.environmentFile != null) cfg.environmentFile;
        Environment =
          [
            "VIVAGYM_LOCALE=${cfg.locale}"
            "PORT=${toString cfg.port}"
            "HOST=${cfg.host}"
            "PUBLIC_URL=${cfg.publicUrl}"
            "LOGIN_RATE_PER_MIN=${toString cfg.loginRatePerMinute}"
            "COOKIE_MAX_AGE_DAYS=${toString cfg.cookieMaxAgeDays}"
            "TRUST_PROXY=${if cfg.trustProxy then "1" else "0"}"
            "OTEL_SERVICE_NAME=${cfg.telemetryServiceName}"
            # No collector endpoint -> keep spans out of journald; logs carry
            # trace_ids for correlation instead.
            "OTEL_TRACES_EXPORTER=${if cfg.telemetryOtlpEndpoint != null then "otlp" else "none"}"
          ]
          ++ lib.optional (cfg.telemetryOtlpEndpoint != null) "OTEL_EXPORTER_OTLP_ENDPOINT=${cfg.telemetryOtlpEndpoint}"
          ++ lib.optional (cfg.clientId != null) "VIVAGYM_CLIENT_ID=${cfg.clientId}"
          ++ lib.optional (cfg.clientSecret != null) "VIVAGYM_CLIENT_SECRET=${cfg.clientSecret}";
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
