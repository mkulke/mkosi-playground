{
  description = "mkosi environment";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-23.11";
    flake-utils.url = "github:numtide/flake-utils";
    uplosi.url = "github:mkulke/uplosi?ref=b31968dda9fb67c0ccc1abf579410d0c1b520007";
  };
  outputs = { self, nixpkgs, flake-utils, uplosi }:
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
          sbsigntool
          systemd
        ] ++ [ uplosi.packages."${system}".default ];
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
