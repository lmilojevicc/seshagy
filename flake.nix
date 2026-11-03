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
      go_1_26_10 = pkgs.go_1_26.overrideAttrs {
        version = "1.26.10";
        src = pkgs.fetchurl {
          url = "https://go.dev/dl/go1.26.10.src.tar.gz";
          hash = "sha256-/WPog7ovK/ypPBZGsx6+9j4rHk0rMXyXidl2po2B828=";
        };
      };
      buildGoModule = pkgs.buildGoModule.override {go = go_1_26_10;};
      seshagy = pkgs.callPackage ./nix/package.nix {inherit buildGoModule version;};
    in {
      inherit seshagy;
      default = seshagy;
    });
  };
}
