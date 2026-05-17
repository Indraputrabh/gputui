# Contributing

Thanks for your interest in `gputui`. This is a small, single-maintainer project, so contributions are welcome but expectations are honest:

* The author runs CI on Linux x86_64 with NVIDIA GPUs. PRs that need other hardware to validate may sit in review until someone with access can test.
* The hint engine is opinionated. New rules need a clear evidence set, a positive and a negative test case, and a sentence in the README.

## Issues

Open an issue if:

* A hint fires when it shouldn't, or fails to fire when it should -- include `--plain` output and, ideally, a snapshot JSONL excerpt.
* A collector misreads NVML or `/proc` -- include `nvidia-smi -q` output and the relevant `/proc` files for triage.
* The TUI breaks at a particular terminal width or font.

## Pull requests

1. Fork and create a branch (`feat/...`, `fix/...`, `docs/...`).
2. Run the full local CI before pushing:
   ```bash
   make ci
   ```
   This runs `gofmt`, `go vet`, `golangci-lint`, `go test -race`, and `go build`.
3. Keep commits focused. Prefer 2-5 small commits over one giant one.
4. Update `README.md` / `ROADMAP.md` / `docs/` if you change behaviour visible to users.
5. New hint rules go under `internal/hints/rules/` with a sibling `_test.go`. Register them in `internal/hints/defaults.go`.
6. New collectors live under `internal/collect/<name>/` with a Linux-specific file (`*_linux.go`) and a stub for non-Linux (`*_other.go`) so the build stays portable.

## Code style

* Plain `gofmt` -- no custom imports order.
* Errors get wrapped with `fmt.Errorf("...: %w", err)` so the caller can inspect them.
* Comments explain *why*, not *what*. Avoid restating the code in prose.
* Avoid global mutable state outside the pipeline package.

## Releasing (maintainer-only)

* Bump the version in `CHANGELOG.md`.
* Tag with `v<major>.<minor>.<patch>` and push the tag.
* GitHub Releases are written manually for now -- there is no release automation.
