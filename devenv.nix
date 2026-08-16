{ pkgs, ... }:
let
  gymqrack = pkgs.callPackage ./package.nix { };
in
{
  name = "gymqrack";

  # Secretspec integration is disabled in devenv.yaml (eval-time validation
  # would break CI), so export the profile/provider the CLI would otherwise get
  # from it, letting `secretspec run`/`check`/`set` resolve against the keyring.
  env = {
    SECRETSPEC_PROFILE = "default";
    SECRETSPEC_PROVIDER = "keyring";
  };

  languages = {
    go.enable = true;
    go.lsp.enable = true;
    java.enable = true;
    java.lsp.enable = true;
  };

  packages = with pkgs; [
    git # needed by the git-hooks (pre-commit) runner
    gymqrack # nix-built server (bin: gymqrack)
    secretspec
  ];

  # Load secrets at runtime so they are only exposed to the processes that
  # need them, not the whole shell (see devenv.sh/integrations/secretspec/).
  processes.dev.exec = "secretspec run -- go run ./cmd/gymqrack";
  processes.server.exec = "secretspec run -- ${gymqrack}/bin/gymqrack";

  # https://devenv.sh/integrations/opencode/
  opencode = {
    enable = true;

    # Attributes written to opencode.jsonc
    settings = { };

    # devenv MCP server so opencode can manage the shell
    mcp.devenv = {
      type = "local";
      command = [
        "devenv"
        "mcp"
      ];
      environment = {
        DEVENV_ROOT = "{env:DEVENV_ROOT}";
      };
    };

    # Global instructions -> .opencode/AGENTS.md
    rules = ''
      # Development Rules

      - Run Go, git, and nix commands inside the devenv shell (`devenv shell ...`); `go` and `staticcheck` are not on the host PATH and the git-hooks fail outside the shell.
      - Run `go test ./...` after Go changes and before finishing a task.
      - Never commit secrets. Load them via `secretspec run -- ...`; tokens only ever live in HttpOnly cookies, never in localStorage.
      - Keep `docs/api.md` in sync with reverse-engineering findings; record endpoint paths, auth flow, and payload formats.
      - Format Nix files with nixfmt-rfc-style and run hooks via `prek` inside the devenv shell.
      - Use conventional commit messages.
    '';

    # Slash commands -> .opencode/commands/
    commands = {
      dev = ''
        # Start the dev server (go run) on port 4567

        ```bash
        devenv up
        ```
      '';
      server = ''
        # Start the nix-built gymqrack binary (secretspec run -- gymqrack)

        ```bash
        devenv up --process server
        ```
      '';
      test = ''
        # Run the test suite

        ```bash
        go test ./...
        ```
      '';
      lint = ''
        # Run gofmt, go vet, and staticcheck

        ```bash
        gofmt -l .
        go vet ./...
        staticcheck ./...
        ```
      '';
      build = ''
        # Build the gymqrack package

        ```bash
        nix build .#gymqrack
        ```
      '';
      secrets = ''
        # Check or set secrets in the macOS keyring via SecretSpec

        Inside the devenv shell run `secretspec check` to list what is present, or
        `secretspec set GYMQRACK_CLIENT_ID` to store a value. Never print secrets.
      '';
    };

    # Subagents -> .opencode/agents/
    agents = {
      code-reviewer = ''
        ---
        description: Reviews Go and Nix code for correctness, security, and maintainability without editing files
        mode: subagent
        temperature: 0.1
        permission:
          edit: deny
          bash:
            "*": deny
            "git *": allow
            "go test*": allow
            "go vet*": allow
            "staticcheck*": allow
            "gofmt*": allow
            "rg *": allow
        ---
        You are an expert code reviewer for gymqrack, a Go stdlib net/http server.
        Check for correctness, security (token handling, cookie flags, rate
        limiting, secrets handling), performance, and adherence to project
        conventions. Report findings with file:line references and suggest fixes
        without applying them.
      '';

      troubleshooter = ''
        ---
        description: Investigates Go build, test, and runtime errors
        mode: subagent
        permission:
          edit: ask
          bash:
            "*": ask
            "go *": allow
            "rg *": allow
        ---
        You are a debugging specialist for a Go server. Reproduce errors with
        `go test ./...`, run targeted searches, find root causes, and propose
        fixes with file:line references. Ask before editing files.
      '';

      api-reverser = ''
        ---
        description: Reverse-engineers the VivaGym app APK and documents the API
        mode: subagent
        permission:
          bash: deny
        ---
        You are a reverse-engineering specialist for the VivaGym Group app
        (`com.myvitale.vivagym.group`). Record endpoint paths, the auth flow, and
        payload formats in docs/api.md, referencing the jadx decompilation under
        apk/jadx-out with file:line citations. Never include credentials.
      '';
    };

    # Skills -> .opencode/skills/<name>/SKILL.md
    skills = {
      "gymqrack-conventions" = ''
        ---
        name: gymqrack-conventions
        description: Follow gymqrack Go server conventions for layout, auth, cookies, and QR handling
        ---
        ## What I do
        Explain and apply gymqrack's Go project conventions: stdlib net/http,
        cmd/gymqrack + internal/{server,vivagym,qr} layout, the OAuth proxy flow
        (client_credentials -> exerp/newAuth), HttpOnly + Secure + SameSite=Lax
        cookie sessions, transparent ~10-minute access-token refresh, and rate
        limiting.
        ## When to use me
        When adding, changing, or debugging handlers, the VivaGym client, or QR logic.
      '';

      "devenv-workflow" = ''
        ---
        name: devenv-workflow
        description: Run gymqrack tooling through the devenv shell
        ---
        ## What I do
        Tooling is managed by devenv. Use `devenv up` for the dev server (go run
        on :4567), `devenv shell` for an interactive session, and the /dev /test
        /lint /build slash commands. git-hooks run gofmt, go vet, staticcheck,
        gotest, nixfmt-rfc-style, deadnix, statix, oxfmt, oxlint, check-json,
        taplo, actionlint, and shellcheck.
        ## When to use me
        Before running any shell command in this project.
      '';

      "secretspec" = ''
        ---
        name: secretspec
        description: Load and manage gymqrack secrets via SecretSpec and the macOS keyring
        ---
        ## What I do
        Secrets (GYMQRACK_CLIENT_ID, GYMQRACK_CLIENT_SECRET) are declared in
        secretspec.toml and stored in the macOS login keychain. Always run
        secretspec commands inside the devenv shell (`devenv shell bash -c '...'`);
        direct keyring access is flaky. Processes load secrets at runtime via
        `secretspec run --`; the devenv.yaml integration is disabled for CI.
        ## When to use me
        Whenever a process needs GYMQRACK_* values or when checking or adding secrets.
      '';
    };
  };

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
    # Format TOML files with taplo.
    taplo.enable = true;
    # Lint GitHub Actions workflow files.
    actionlint.enable = true;
    # Lint shell scripts.
    shellcheck.enable = true;
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
