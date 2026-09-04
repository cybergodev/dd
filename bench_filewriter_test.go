package dd

// BenchmarkFileWriterWriteLine measures the FileWriter.Write hot path
// (non-rotating steady state). Added during the P-002 concurrency audit to
// verify the rotation-callback fix has no measurable write overhead.

import (
	"path/filepath"
	"testing"
)

func BenchmarkFileWriterWriteLine(b *testing.B) {
	fw, err := NewFileWriter(filepath.Join(b.TempDir(), "bench.log"), FileWriterConfig{MaxSizeMB: 100})
	if err != nil {
		b.Fatal(err)
	}
	defer fw.Close()

	data := []byte("[2026-08-30T12:00:00.000+08:00  INFO] bench_test.go:20 benchmark log line user=abc123 request_id=def456")
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := fw.Write(data); err != nil {
			b.Fatal(err)
		}
	}
}
