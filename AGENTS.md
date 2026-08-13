# Agent notes

## Committing

Run git commands inside the devenv shell (`devenv shell git commit ...`). The
`staticcheck` pre-commit hook invokes `go` from PATH, which is only present in
the devenv shell; outside it, the hook fails even though staticcheck passes.
