{
  description = "cream, switch CLI logins between named profiles";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      lib = nixpkgs.lib;
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAll = f: lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      version = lib.trim (builtins.readFile ./VERSION);
    in
    {
      packages = forAll (pkgs: {
        default = pkgs.buildGoModule {
          pname = "cream";
          inherit version;
          src = ./.;
          # update after every go.mod or go.sum change, nix build prints the right hash
          vendorHash = lib.fakeHash;
          env.CGO_ENABLED = 0;
          ldflags = [
            "-s"
            "-w"
            "-X main.version=v${version}"
          ];
          # the git rule and doctor tests run real git
          nativeCheckInputs = [ pkgs.git ];
          preCheck = "export HOME=$TMPDIR";
          meta = {
            description = "Switch CLI logins (Claude Code, Codex, gh, glab, git identity) between named profiles";
            homepage = "https://github.com/gdamigos/cream";
            license = lib.licenses.mit;
            mainProgram = "cream";
          };
        };
      });
    };
}
