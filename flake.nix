{
  description = "Agent-aware terminal dashboard for tmux and herdr";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs";

  outputs = {
    self,
    nixpkgs,
  }: let
    systems = [
      "x86_64-linux"
      "aarch64-linux"
      "aarch64-darwin"
    ];
    genAttrs = names: f:
      builtins.listToAttrs (map (name: {
          inherit name;
          value = f name;
        })
        names);
    version = "0-unstable-${builtins.substring 0 8 (self.lastModifiedDate or "19700101")}";
  in {
    packages = genAttrs systems (system: let
      pkgs = import nixpkgs {
        inherit system;
      };
      seshagy = pkgs.callPackage ./nix/package.nix {inherit version;};
    in {
      inherit seshagy;
      default = seshagy;
    });
  };
}
