{
  description = "devshell";
  nixConfig.bash-prompt = "[nix] ";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    { nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (system: {
      devShell =
        with nixpkgs.legacyPackages.${system};
        mkShell {
          nativeBuildInputs = with pkgs; [
            go_1_23
            gopls
          ];
          shellHook = ''
            for p in $NIX_PROFILES; do
              GOPATH="$p/share/go:$GOPATH"
            done
          '';
        };
    });
}
