{ pkgs }:

let
  synapse = import ./synapse.nix { inherit pkgs; };
  healthcheck = pkgs.writeShellScriptBin "synapse-worker-healthcheck" ''
    kill -0 1
  '';
in
pkgs.dockerTools.buildLayeredImage {
  name = "synapse-worker";
  tag = "latest";

  contents = [
    synapse
    healthcheck
    pkgs.cacert
  ];

  config = {
    Entrypoint = [ "${synapse}/bin/synapse-worker" ];
    Env = [
      "RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672"
      "S3_ENDPOINT=http://minio:9000"
      "S3_ACCESS_KEY=minioadmin"
      "S3_HOT_BUCKET=synapse-hot"
      "S3_COLD_BUCKET=synapse-cold"
      "STORAGE_BACKEND=s3"
      "SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt"
    ];
  };
}
