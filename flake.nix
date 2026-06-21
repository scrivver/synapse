{
  description = "Flake for the synapse project, includes development environment and infrastructure service definitions.";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
	let
	pkgs = import nixpkgs {
	inherit system;
	config.allowUnfree = true;
	};
	rabbitmqInfra = import ./infra/rabbitmq.nix { inherit pkgs; };
	minioInfra = import ./infra/minio.nix { inherit pkgs; };
	yamlFormat = pkgs.formats.yaml {};
	processComposeConfig = yamlFormat.generate "process-compose.yaml" {
	  version = "0.5";
	  processes = rabbitmqInfra.processes // minioInfra.processes;
	};
	infraShell = import ./shells/infra.nix { inherit pkgs processComposeConfig; };
	devShellNix = import ./shells/dev.nix { inherit pkgs infraShell; };
	in
	{
	packages = {
	  synapse = import ./nix/synapse.nix { inherit pkgs; };
	  worker-container = import ./nix/worker-container.nix { inherit pkgs; };
	  reconciler-container = import ./nix/reconciler-container.nix { inherit pkgs; };
	  default = self.packages.${system}.synapse;
	};

	devShells = rec {
	  infra   = infraShell;
	  dev     = devShellNix;
	  default = dev;
	};
	}
  );
}
