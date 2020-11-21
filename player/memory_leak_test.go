package player

import (
	"testing"
)

func BenchmarkMemoryLeak(b *testing.B) {
	b.ReportAllocs()

}
