# Agent notes

## Committing

Run git commands inside the devenv shell (`devenv shell git commit ...`). The
`staticcheck` pre-commit hook invokes `go` from PATH, which is only present in
the devenv shell; outside it, the hook fails even though staticcheck passes.

## SecretSpec / macOS keyring

Secrets (`GYMQRACK_CLIENT_ID`, `GYMQRACK_CLIENT_SECRET`) are declared in
`secretspec.toml` and stored in the macOS login keychain (service
`secretspec/gymqrack/default/<NAME>`). Non-secret config (`GYMQRACK_LOCALE`,
`PORT`, `PUBLIC_URL`) has committed defaults in `secretspec.toml`. devenv
processes load secrets at runtime via `secretspec run --` (see devenv.nix).

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

