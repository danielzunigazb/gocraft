//go:build windows

package client

import (
	"math"
	"testing"
	"time"
)

// The winmm sink takes audio at the rate the device plays it (that wait is the pacing) and lets go of the
// device on close, also when a Write is waiting. Skipped where there is no output device.
func TestWaveSinkPacesAndCloses(t *testing.T) {
	s, err := openSink()
	if err != nil {
		t.Skip("no output device: " + err.Error())
	}
	chunk := make([]byte, 4*mixChunk) // silence: the test makes no noise
	for i := 0; i < 4*mixChunk; i += 4 {
		v := int16(40 * math.Sin(float64(i)/4*0.05)) // inaudible
		chunk[i], chunk[i+1], chunk[i+2], chunk[i+3] = byte(v), byte(uint16(v)>>8), byte(v), byte(uint16(v)>>8)
	}
	const chunks = 30 // 600 ms of audio
	t0 := time.Now()
	for i := 0; i < chunks; i++ {
		if n, err := s.w.Write(chunk); err != nil || n != len(chunk) {
			t.Fatalf("write %d: %d bytes, %v", i, n, err)
		}
	}
	// All but the queue's worth must have been played by the time the last Write returned.
	want := time.Duration(chunks-waveQueue()) * 20 * time.Millisecond
	if d := time.Since(t0); d < want*8/10 {
		t.Fatalf("%d chunks went in %v: Write does not wait for the device (want at least %v)", chunks, d, want)
	}
	// A Write stuck on a full queue is released by close.
	done := make(chan error, 1)
	go func() {
		var err error
		for err == nil {
			_, err = s.w.Write(chunk)
		}
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	s.close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("close did not release the waiting Write")
	}
	s.close() // idempotent
}
