package pipeline

import (
	"context"
	"testing"
)

// BenchmarkPipelineCollect measures the full per-sample wall time of the
// demo pipeline (no NVML / /proc dependency) so results are portable
// across developer machines and CI runners.
func BenchmarkPipelineCollect(b *testing.B) {
	pipe, err := New("nvml", true)
	if err != nil {
		b.Fatalf("new pipeline: %v", err)
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := pipe.Collect(ctx); err != nil {
			b.Fatalf("collect: %v", err)
		}
	}
}
