{
  description = "PostNest — Go-based mail platform";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.11";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];
    in
    flake-utils.lib.eachSystem supportedSystems (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        postnest = pkgs.buildGoModule {
          pname = "postnest";
          version = "0.1.0";
          src = self;
          vendorHash = null; # Uses vendor directory or will compute on first build
          subPackages = [ "cmd/server" "cmd/worker" "cmd/migrate" ];
          ldflags = [ "-s" "-w" ];
        };
      in
      {
        packages = {
          postnest-server = postnest.overrideAttrs (old: { subPackages = [ "cmd/server" ]; });
          postnest-worker = postnest.overrideAttrs (old: { subPackages = [ "cmd/worker" ]; });
          postnest-migrate = postnest.overrideAttrs (old: { subPackages = [ "cmd/migrate" ]; });
          default = postnest;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            golangci-lint
            air
            postgresql_16
            redis
            go-migrate
          ];
        };
      }) // {
        nixosModules.postnest = import ./nix/module.nix { inherit self; };
      };
}
