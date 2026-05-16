{ self }:

{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.postnest;
  tomlFormat = pkgs.formats.toml {};
in
{
  options.services.postnest = {
    enable = mkEnableOption "PostNest mail platform";

    server.enable = mkEnableOption "PostNest server" // { default = true; };
    worker.enable = mkEnableOption "PostNest worker" // { default = true; };

    database = {
      host = mkOption {
        type = types.str;
        default = "localhost";
        description = "PostgreSQL host address.";
      };
      port = mkOption {
        type = types.port;
        default = 5432;
        description = "PostgreSQL port.";
      };
      name = mkOption {
        type = types.str;
        default = "postnest";
        description = "PostgreSQL database name.";
      };
      user = mkOption {
        type = types.str;
        default = "postnest";
        description = "PostgreSQL user name.";
      };
      passwordFile = mkOption {
        type = types.nullOr types.path;
        default = null;
        description = "Path to a file containing the PostgreSQL password.";
      };
    };

    redis = {
      enable = mkEnableOption "Redis for PostNest" // { default = true; };
      host = mkOption {
        type = types.str;
        default = "localhost";
      };
      port = mkOption {
        type = types.port;
        default = 6379;
      };
    };

    settings = mkOption {
      type = tomlFormat.type;
      default = {};
      description = "Extra TOML settings merged into /etc/postnest/postnest.conf.";
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.database.passwordFile != null;
        message = "services.postnest.database.passwordFile must be set.";
      }
    ];

    # PostgreSQL integration
    services.postgresql = {
      enable = true;
      ensureDatabases = [ cfg.database.name ];
      ensureUsers = [
        {
          name = cfg.database.user;
          ensureDBOwnership = true;
        }
      ];
    };

    # Redis integration
    services.redis.servers.postnest = mkIf cfg.redis.enable {
      enable = true;
      bind = cfg.redis.host;
      port = cfg.redis.port;
    };

    # Generated config file
    environment.etc."postnest/postnest.conf".source = tomlFormat.generate "postnest.conf" (
      {
        config_version = 1;
        server = {
          http_addr = ":8080";
          imap_addr = ":143";
          smtp_addr = ":587";
        };
        database = {
          dsn = "postgres://${cfg.database.user}@${cfg.database.host}:${toString cfg.database.port}/${cfg.database.name}?sslmode=disable";
        };
        redis = {
          url = "redis://${cfg.redis.host}:${toString cfg.redis.port}/0";
        };
      }
      // cfg.settings
    );

    # Systemd services
    systemd.services.postnest-server = mkIf cfg.server.enable {
      description = "PostNest Server";
      after = [ "network-online.target" "postgresql.service" ] ++ optional cfg.redis.enable "redis-postnest.service";
      wantedBy = [ "multi-user.target" ];
      serviceConfig = {
        Type = "simple";
        ExecStart = "${self.packages.${pkgs.system}.postnest-server}/bin/server";
        Restart = "on-failure";
        User = "postnest";
        Group = "postnest";
        Environment = [ "POSTNEST_CONFIG_PATH=/etc/postnest/postnest.conf" ];
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = "/var/lib/postnest";
      };
    };

    systemd.services.postnest-worker = mkIf cfg.worker.enable {
      description = "PostNest Worker";
      after = [ "network-online.target" "postgresql.service" "postnest-server.service" ] ++ optional cfg.redis.enable "redis-postnest.service";
      wantedBy = [ "multi-user.target" ];
      serviceConfig = {
        Type = "simple";
        ExecStart = "${self.packages.${pkgs.system}.postnest-worker}/bin/worker";
        Restart = "on-failure";
        User = "postnest";
        Group = "postnest";
        Environment = [ "POSTNEST_CONFIG_PATH=/etc/postnest/postnest.conf" ];
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
      };
    };

    users.users.postnest = {
      isSystemUser = true;
      group = "postnest";
      home = "/var/lib/postnest";
      createHome = true;
    };
    users.groups.postnest = {};
  };
}
