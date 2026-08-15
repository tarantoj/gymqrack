# Agent notes

## Committing

Run git commands inside the devenv shell (`devenv shell git commit ...`). The
`staticcheck` pre-commit hook invokes `go` from PATH, which is only present in
the devenv shell; outside it, the hook fails even though staticcheck passes.

## Git hooks (devenv)

Hooks are declared under `git-hooks.hooks` in `devenv.nix`, not committed
directly. The generated `.pre-commit-config.yaml` is a gitignored symlink into
the nix store — never edit it by hand.

Adding a hook:

- Enable it as `git-hooks.hooks.<name>.enable = true;`. Hook names come from
  cachix/git-hooks.nix (see its `modules/hooks.nix` for available names and
  settings; e.g. `taplo`, `nixfmt-rfc-style`, `statix`, `oxfmt`).
- Some hooks take settings: `git-hooks.hooks.<name>.settings = { ... };`, and
  some need per-hook excludes (see the existing `oxfmt`/`oxlint` exclusions for
  vendored `htmx.min.js`).
- Re-enter the devenv shell (`devenv shell`) so the config regenerates and the
  hook installs; verify with `prek run <name> --files <file>`.

Maintaining:

- Most formatter hooks run in write/fix mode (e.g. `taplo fmt`, nixfmt) and
  auto-modify files, failing the commit. If a commit fails with "files were
  modified by this hook", `git add` the changes and commit again.
- Nix formatters reformat `devenv.nix` itself, so after editing it, run
  `prek run nixfmt-rfc-style --files devenv.nix` (or let the hook fix it and
  re-add) before committing.
- Run hooks / prek only inside the devenv shell; the hook binaries (prek,
  nixfmt, staticcheck) are not on the host PATH.

## SecretSpec / macOS keyring

Secrets (`GYMQRACK_CLIENT_ID`, `GYMQRACK_CLIENT_SECRET`) are declared in
`secretspec.toml` and stored in the macOS login keychain (service
`secretspec/gymqrack/default/<NAME>`). Non-secret config (`GYMQRACK_LOCALE`,
`PORT`, `PUBLIC_URL`) has committed defaults in `secretspec.toml`. devenv
processes load secrets at runtime via `secretspec run --` (see devenv.nix).

The secretspec integration is DISABLED in `devenv.yaml`
(`secretspec.enable = false`): enabling it makes devenv validate required
secrets at eval time, which breaks CI (no keyring on runners). Instead
`devenv.nix` exports `SECRETSPEC_PROFILE=default` and
`SECRETSPEC_PROVIDER=keyring` in `env`, so the CLI resolves the same way.

Findings from setup:

- Always run secretspec commands inside the devenv shell (`devenv shell bash
  -c '...'`), never directly. Inside the shell, `secretspec check` reliably
  reports `5 found, 0 missing`; directly, the macOS keyring access is flaky
  and intermittently fails with `Keyring error: No default store has been set,
  so cannot search or create entries` even though the data is present and the
  keychain is unlocked.
- To confirm a value is stored, use `security find-generic-password -s
  "secretspec/gymqrack/default/<NAME>" <login keychain>` (reliable) instead of
  `secretspec get`.
- The manifest's `require_reason` policy (default `"agents"`) forces agent
  processes to pass `--reason "<why>"` to secretspec commands; interactive
  sessions are unaffected.
- When running secretspec commands, export `SECRETSPEC_PROFILE=default` (the
  user-global default profile is `development`, which the manifest does not
  define).

