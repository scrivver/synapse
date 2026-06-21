{ pkgs }:

pkgs.buildGoModule {
  pname = "synapse";
  version = "0.1.0";

  src = ../.;

  vendorHash = "sha256-TUmNp5hZotCyeou3/y6Hyo9dD9FIobyRc9KJJgMU1no=";
  subPackages = [
    "cmd/synapse-worker"
    "cmd/synapse-reconciler"
    "cmd/synapse-metagen"
  ];

  meta = {
    description = "Synapse reconciliation and transfer commands";
    mainProgram = "synapse-worker";
  };
}
