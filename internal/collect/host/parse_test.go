package host

import (
	"math"
	"testing"
)

func TestParseLoadAvg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []float64
		wantErr bool
	}{
		{
			name:  "typical",
			input: "4.20 3.80 3.20 2/1234 56789\n",
			want:  []float64{4.20, 3.80, 3.20},
		},
		{
			name:  "zeros",
			input: "0.00 0.00 0.00 1/100 1",
			want:  []float64{0, 0, 0},
		},
		{
			name:    "too few fields",
			input:   "4.20 3.80",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "bad float",
			input:   "abc 3.80 3.20 1/1 1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseLoadAvg(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("length mismatch: got %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if math.Abs(got[i]-tt.want[i]) > 0.001 {
					t.Errorf("field %d: got %f, want %f", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseMeminfo(t *testing.T) {
	t.Parallel()

	input := `MemTotal:       65536000 kB
MemFree:        10000000 kB
MemAvailable:   30000000 kB
Buffers:          500000 kB
Cached:         20000000 kB
SwapTotal:       8000000 kB
SwapFree:        6000000 kB
`
	result := ParseMeminfo(input)

	wantMemTotal := uint64(65536000) * 1024
	wantMemAvail := uint64(30000000) * 1024
	wantSwapTotal := uint64(8000000) * 1024
	wantSwapUsed := uint64(2000000) * 1024

	if result.MemTotal != wantMemTotal {
		t.Errorf("MemTotal: got %d, want %d", result.MemTotal, wantMemTotal)
	}
	if result.MemAvailable != wantMemAvail {
		t.Errorf("MemAvailable: got %d, want %d", result.MemAvailable, wantMemAvail)
	}
	if result.SwapTotal != wantSwapTotal {
		t.Errorf("SwapTotal: got %d, want %d", result.SwapTotal, wantSwapTotal)
	}
	if result.SwapUsed != wantSwapUsed {
		t.Errorf("SwapUsed: got %d, want %d", result.SwapUsed, wantSwapUsed)
	}
}

func TestParseMeminfoNoSwap(t *testing.T) {
	t.Parallel()

	input := `MemTotal:       32000000 kB
MemAvailable:   16000000 kB
SwapTotal:             0 kB
SwapFree:              0 kB
`
	result := ParseMeminfo(input)

	if result.SwapTotal != 0 {
		t.Errorf("SwapTotal: got %d, want 0", result.SwapTotal)
	}
	if result.SwapUsed != 0 {
		t.Errorf("SwapUsed: got %d, want 0", result.SwapUsed)
	}
}

func TestParseMeminfoEmpty(t *testing.T) {
	t.Parallel()
	result := ParseMeminfo("")
	if result.MemTotal != 0 || result.MemAvailable != 0 {
		t.Errorf("expected zeros for empty input, got %+v", result)
	}
}

func TestParseCPUStatLine(t *testing.T) {
	t.Parallel()

	fields := []string{"cpu", "100", "20", "30", "800", "10", "5", "3"}
	sample, err := ParseCPUStatLine(fields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantTotal := uint64(100 + 20 + 30 + 800 + 10 + 5 + 3)
	if sample.Total != wantTotal {
		t.Errorf("Total: got %d, want %d", sample.Total, wantTotal)
	}
	if sample.User != 100 {
		t.Errorf("User: got %d, want 100", sample.User)
	}
	if sample.Iowait != 10 {
		t.Errorf("Iowait: got %d, want 10", sample.Iowait)
	}
}

func TestParseCPUStatLineTooFewFields(t *testing.T) {
	t.Parallel()

	fields := []string{"cpu", "100", "20"}
	_, err := ParseCPUStatLine(fields)
	if err == nil {
		t.Fatal("expected error for too few fields")
	}
}

func TestParseCPUStatLineBadValue(t *testing.T) {
	t.Parallel()

	fields := []string{"cpu", "abc", "20", "30", "800", "10", "5", "3"}
	_, err := ParseCPUStatLine(fields)
	if err == nil {
		t.Fatal("expected error for bad value")
	}
}

func TestComputeCPUDelta(t *testing.T) {
	t.Parallel()

	prev := CPUSample{
		Total: 1000, User: 100, Nice: 10, Sys: 50,
		Idle: 800, Iowait: 20, IRQ: 10, SoftIRQ: 10,
	}
	cur := CPUSample{
		Total: 2000, User: 300, Nice: 20, Sys: 100,
		Idle: 1500, Iowait: 40, IRQ: 20, SoftIRQ: 20,
	}

	delta := ComputeCPUDelta(prev, cur)

	deltaTotal := float64(cur.Total - prev.Total) // 1000

	wantUser := 100.0 * float64((300-100)+(20-10)) / deltaTotal // 21%
	wantSys := 100.0 * float64((100-50)+(20-10)+(20-10)) / deltaTotal
	wantIowait := 100.0 * float64(40-20) / deltaTotal

	if math.Abs(delta.User-wantUser) > 0.01 {
		t.Errorf("User: got %f, want %f", delta.User, wantUser)
	}
	if math.Abs(delta.Sys-wantSys) > 0.01 {
		t.Errorf("Sys: got %f, want %f", delta.Sys, wantSys)
	}
	if math.Abs(delta.Iowait-wantIowait) > 0.01 {
		t.Errorf("Iowait: got %f, want %f", delta.Iowait, wantIowait)
	}
}

func TestComputeCPUDeltaZeroTotal(t *testing.T) {
	t.Parallel()

	s := CPUSample{Total: 500}
	delta := ComputeCPUDelta(s, s)

	if delta.User != 0 || delta.Sys != 0 || delta.Iowait != 0 {
		t.Errorf("expected zero delta for identical samples, got %+v", delta)
	}
}
