{
  description = "bleamd - Markdown Renderer & Search for the terminal";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        
        # Version information.
        #
        # Flake evaluation is pure and Nix does not expose git tags (self gives
        # us rev/shortRev/revCount and nothing more), so the release number
        # lives in ./VERSION -- the file the git tag is cut from. Commit
        # identity does come from git, so a build always names its own source.
        version = nixpkgs.lib.fileContents ./VERSION;
        gitCommit = self.rev or self.dirtyRev or "unknown";
        gitShortRev = self.shortRev or self.dirtyShortRev or "unknown";

      in
      {
        packages = {
          default = self.packages.${system}.bleamd;
          
          bleamd = pkgs.buildGoModule {
            pname = "bleamd";
            inherit version;
            
            src = pkgs.lib.cleanSource ./.;
            
            # Generate vendor hash with: nix run nixpkgs#nix-prefetch-git -- --url . --fetch-submodules
            # Or let nix tell you the correct hash on first build
            vendorHash = "sha256-XMl/NmD/Ki2Jx9glICHKFMv4Sjk6MOlxmP8WANtcbjc=";
            
            # Add version information as build flags
            ldflags = [
              "-s"
              "-w"
              "-X main.GitCommit=${gitCommit}"
              # Always carries the commit, so an untagged build never claims to
              # be the release itself.
              "-X main.GitLastTag=v${version}+${gitShortRev}"
              # The flake cannot see tags, so never assert an exact-tag match.
              "-X main.GitExactTag=undefined"
            ];
            
            # Ensure binary is named bleamd (Go may name it based on directory/module)
            # This is a safety check in case the build process creates 'mdr'
            postInstall = ''
              if [ -f $out/bin/mdr ] && [ ! -f $out/bin/bleamd ]; then
                mv $out/bin/mdr $out/bin/bleamd
              fi

              install -Dm644 ${./bleamd-icon.svg} $out/share/icons/hicolor/scalable/apps/bleamd-icon.svg

              install -Dm644 ${./bleamd.desktop} $out/share/applications/bleamd.desktop
            '';
            
            # Disable tests that might require network access
            doCheck = false;
            
            meta = with pkgs.lib; {
              description = "A standalone Markdown renderer for the terminal with search functionality";
              homepage = "https://github.com/guttermonk/bleamd";
              license = licenses.mit;
              maintainers = [];
              mainProgram = "bleamd";
            };
          };
        };
        
        apps = {
          default = self.apps.${system}.bleamd;
          
          bleamd = {
            type = "app";
            program = "${self.packages.${system}.bleamd}/bin/bleamd";
          };
        };
        
        devShells = {
          default = pkgs.mkShell {
            buildInputs = with pkgs; [
              # Go development
              go
              gopls
              go-tools
              golangci-lint
              
              # Build tools
              gnumake
              git
              
              # Optional: for cross-compilation
              gox
              
              # Helpful tools
              entr  # for file watching
              ripgrep  # for searching
            ];
            
            shellHook = ''
              echo "bleamd development environment"
              echo "Available commands:"
              echo "  go build       - Build the project"
              echo "  go run . FILE  - Run bleamd with a markdown file"
              echo "  make build     - Build using Makefile"
              echo "  nix build      - Build with nix"
              echo "  nix run        - Run the built version"
              echo ""
              echo "Go version: $(go version)"
            '';
          };
        };
        
        # Legacy support for nix-shell
        devShell = self.devShells.${system}.default;
      });
}
