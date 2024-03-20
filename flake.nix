{
  description = "mkosi environment";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-23.11";
    flake-utils.url = "github:numtide/flake-utils";
  };
  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };
        name = "mkosi";
        project-buildInputs = with pkgs; [
          dnf5
          mkosi-full
          mtools
          systemd
        ];
      in {
        devShells.default = pkgs.mkShell {
          NIX_HARDENING_ENABLE = "";
          buildInputs = project-buildInputs;
          shellHook = ''
            source ~/.profile
            export PS1="$(sed 's|\\u@\\h|(nix:${name})|g' <<< $PS1)"
          '';
        };
        formatter = pkgs.nixfmt;
      });
}
