{ pkgs, processComposeConfig }:

pkgs.mkShell {
  name = "synapse-infra-shell";
  buildInputs = [
    pkgs.rabbitmq-server
    pkgs.minio
    pkgs.minio-client
    pkgs.process-compose
    pkgs.python3
    pkgs.curl
  ];

  shellHook = ''
    export SHELL=${pkgs.bash}/bin/bash
    export PATH="$PWD/bin:$PATH"

    export DATA_DIR="$PWD/.data"
    mkdir -p "$DATA_DIR"
    mkdir -p "$DATA_DIR/rabbitmq"
    mkdir -p "$DATA_DIR/minio"
    mkdir -p "$DATA_DIR/storage/synapse-hot"
    mkdir -p "$DATA_DIR/storage/synapse-cold"

    # Generate process-compose config
    cp -f ${processComposeConfig} "$DATA_DIR/process-compose.yaml"

    # Process-compose unix socket path
    export PC_SOCKET="$DATA_DIR/process-compose.sock"

    # Export port file paths so other services can read the dynamic ports
    export RABBITMQ_AMQP_PORT_FILE="$DATA_DIR/rabbitmq/amqp_port"
    export RABBITMQ_MGMT_PORT_FILE="$DATA_DIR/rabbitmq/mgmt_port"

    # MinIO S3 port files
    export MINIO_API_PORT_FILE="$DATA_DIR/minio/api_port"
    export MINIO_CONSOLE_PORT_FILE="$DATA_DIR/minio/console_port"
  '';
}
