{ pkgs, infraShell }:

pkgs.mkShell {
  name = "synapse-dev-shell";
  inputsFrom = [ infraShell ];
  buildInputs = [
    pkgs.go
    pkgs.gopls
    pkgs.gotools
    pkgs.air
  ];
}
