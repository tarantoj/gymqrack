{ pkgs, ... }:
let
  gymqrack = pkgs.callPackage ./package.nix { };
in
{
  name = "gymqrack";

  languages.go.enable = true;
  languages.go.lsp.enable = true;

  packages = with pkgs; [
    git # needed by the git-hooks (pre-commit) runner
    gymqrack # nix-built server (bin: gymqrack)
    secretspec
  ];

  # Load secrets at runtime so they are only exposed to the processes that
  # need them, not the whole shell (see devenv.sh/integrations/secretspec/).
  processes.dev.exec = "secretspec run -- go run ./cmd/gymqrack";
  processes.server.exec = "secretspec run -- ${gymqrack}/bin/gymqrack";

  git-hooks.hooks = {
    # Format Go source with gofmt.
    gofmt.enable = true;
    # Run `go vet` correctness checks.
    govet.enable = true;
    # Run the staticcheck static analyzer.
    staticcheck.enable = true;
    # Run the test suite for modified packages.
    gotest.enable = true;
    # Format Nix files (RFC 166 style).
    nixfmt-rfc-style.enable = true;
    # Find dead code in Nix files.
    deadnix.enable = true;
    # Lint Nix files.
    statix.enable = true;
    # Format JavaScript/TypeScript.
    oxfmt.enable = true;
    # Lint JavaScript/TypeScript.
    oxlint.enable = true;
    # Validate JSON syntax.
    check-json.enable = true;
    # Lint GitHub Actions workflow files.
    actionlint.enable = true;
    # htmx.min.js is a vendored minified library; never reformat or lint it.
    oxfmt.excludes = [ "htmx\\.min\\.js$" ];
    oxlint.excludes = [ "htmx\\.min\\.js$" ];
  };

  enterShell = ''
    echo "Gymqrack dev shell"
    echo "  secrets    : secretspec set GYMQRACK_CLIENT_ID (once, keyring)"
    echo "  dev server : devenv up            (secretspec run -- go run ./cmd/gymqrack)"
    echo "  nix build  : devenv up --process server   (secretspec run -- gymqrack)"
  '';
}
