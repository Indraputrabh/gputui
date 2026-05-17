.PHONY: build test vet lint fmt ci clean bench bench-cpu perf-csv demo-gif

BIN_DIR := ./bin
BENCH_DIR := ./bench_results

build:
	go build -o $(BIN_DIR)/gputui ./cmd/gputui
	go build -o $(BIN_DIR)/gputui-agent ./cmd/gputui-agent

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Files not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

ci: fmt vet lint test build

# bench runs Go benchmarks across the collection pipeline, hints engine,
# and proc/host packages with per-op allocation reporting. Use this to
# compare before/after when iterating on optimisations.
bench:
	@mkdir -p $(BENCH_DIR)
	go test -run=^$$ -bench=. -benchmem -benchtime=1s \
		./internal/collect/... \
		./internal/hints/... \
		./internal/record/... \
		| tee $(BENCH_DIR)/bench.txt

# bench-cpu captures a CPU profile for the full Pipeline.Collect hot path
# (demo mode + real hints evaluation). The resulting pprof file can be
# opened with `go tool pprof bench_results/pipeline.cpu.pprof`.
bench-cpu:
	@mkdir -p $(BENCH_DIR)
	go test -run=^$$ -bench=BenchmarkPipelineCollect -benchtime=3s \
		-cpuprofile=$(BENCH_DIR)/pipeline.cpu.pprof \
		-memprofile=$(BENCH_DIR)/pipeline.mem.pprof \
		./internal/collect/pipeline/...

# perf-csv runs the demo pipeline for N samples, records per-sample wall
# time, and writes a CSV suitable for comparing performance before vs
# after an optimisation (used by remote CI scripts).
perf-csv:
	@mkdir -p $(BENCH_DIR)
	go run ./cmd/gputui-perf -samples=$${PERF_SAMPLES:-100} \
		-out=$(BENCH_DIR)/pipeline_latency.csv
	@echo "wrote $(BENCH_DIR)/pipeline_latency.csv"

clean:
	rm -rf $(BIN_DIR) $(BENCH_DIR)

# demo-gif rebuilds the binary and re-records docs/demo.gif from
# demo.tape via vhs (https://github.com/charmbracelet/vhs).
# Requires `brew install vhs` on macOS.
demo-gif: build
	@command -v vhs >/dev/null 2>&1 || { \
		echo "error: vhs is not installed (brew install vhs)" >&2; \
		exit 1; \
	}
	vhs demo.tape
