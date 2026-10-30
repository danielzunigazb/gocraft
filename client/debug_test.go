package client

import (
	"strings"
	"testing"
)

func TestSamplesPercentiles(t *testing.T) {
	timersKeep = true
	defer func() { timersKeep = false }()
	var s samples
	if s.recent(0.99) != 0 || !strings.HasSuffix(s.line("x"), "no samples") {
		t.Fatalf("an empty phase reports nothing: %v %q", s.recent(0.99), s.line("x"))
	}
	for i := 1; i <= 1000; i++ {
		s.add(float32(i))
	}
	// The ring holds the last 300 (701..1000); the run keeps all of them.
	if got := s.recent(0.5); got != 851 {
		t.Fatalf("median of the last 300: got %v, want 851", got)
	}
	if got := s.recent(0.99); got != 998 {
		t.Fatalf("p99 of the last 300: got %v, want 998", got)
	}
	if len(s.all) != 1000 || s.max != 1000 || s.sum != 500500 {
		t.Fatalf("run totals: %d samples, max %v, sum %v", len(s.all), s.max, s.sum)
	}
	if got := pctOf(s.all, 0.5); got != 501 {
		t.Fatalf("p50 of the run: got %v, want 501", got)
	}
	if got := pctOf(s.all, 0.999); got != 1000 {
		t.Fatalf("p99.9 of the run: got %v, want 1000", got)
	}
	timersKeep = false
	var u samples
	u.add(1)
	if len(u.all) != 0 {
		t.Fatalf("outside a benchmark nothing is kept, got %d", len(u.all))
	}
}
