# Legacy shell.nix for compatibility with nix-shell
# This file provides backwards compatibility for users who prefer nix-shell over nix develop

{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
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
    echo "bleamd development environment (nix-shell)"
    echo "Available commands:"
    echo "  go build       - Build the project"
    echo "  go run . FILE  - Run bleamd with a markdown file"
    echo "  make build     - Build using Makefile"
    echo ""
    echo "Go version: $(go version)"
    echo ""
    echo "Note: Consider using 'nix develop' with flakes for a better experience"
  '';
}