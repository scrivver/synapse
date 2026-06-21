{ pkgs }:

let
  synapse = import ./synapse.nix { inherit pkgs; };
  healthcheck = pkgs.writeShellScriptBin "synapse-reconciler-healthcheck" ''
    kill -0 1
  '';
in
pkgs.dockerTools.buildLayeredImage {
  name = "synapse-reconciler";
  tag = "latest";

  contents = [
    synapse
    healthcheck
    pkgs.cacert
  ];

  config = {
    Entrypoint = [ "${synapse}/bin/synapse-reconciler" ];
    Env = [
      "RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672"
      "ENGRAM_API_URL=http://engram-api:8081/api"
      "RECONCILE_INTERVAL=30s"
      "STORAGE_BACKEND=s3"
      "SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt"
    ];
  };
}
