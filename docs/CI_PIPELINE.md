# CI Pipeline

`gputui` runs a single GitHub Actions workflow on every push and pull request to `main`: [.github/workflows/ci.yml](../.github/workflows/ci.yml).

It runs entirely on `ubuntu-latest` GitHub-hosted runners. There are no secrets, no SSH, and no GPU dependency -- the suite uses standalone `--demo` data paths and unit tests for everything that needs hardware coverage.

## Jobs

| Job | What it does |
|-----|-------------|
| **Build** | `go build ./...` |
| **Test**  | `go test -race -count=1 ./...` |
| **Vet**   | `go vet ./...` |
| **Lint**  | `golangci-lint run ./...` (installed from source so it tracks the Go toolchain in `go.mod`) |
| **Format**| `gofmt -l .` must produce no output |

All five jobs run in parallel.

## Running locally

The same checks are wired into the `Makefile`:

```bash
make ci         # fmt + vet + lint + test + build
make build      # binaries in ./bin/
make test       # go test -race ./...
```

`make bench` and `make bench-cpu` produce benchmark artefacts under `bench_results/` (gitignored).

## Why no live-GPU job?

NVML and `/proc` paths are exercised through unit tests with table-driven fixtures and through a synthetic `--demo` snapshot pipeline that drives every hint rule. CI runs on a generic Linux runner with no NVIDIA driver installed and still covers all hint rules end-to-end.

If you want to validate against a real GPU node, run `./bin/gputui --plain` over SSH or wire it into your own monitoring stack -- there's intentionally no project-owned GPU CI to depend on.
