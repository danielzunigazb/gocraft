package client

import "runtime"

// renderCounters count what a frame asks of the GL driver, for F3 and the benchmark: they do not
// depend on how fast the GPU is, so they compare builds even where the frame rate cannot.
type renderCounters struct {
	draws       int // draw calls of the world's columns and water (a multi-draw counts once)
	drawRanges  int // quad ranges those calls cover
	entDraws    int // draw calls of per-frame geometry: items, mobs, particles, hand, chests
	entBytes    int // vertex bytes streamed for them
	uploads     int // column meshes sent to the GPU
	uploadBytes int
}

var rc renderCounters

// benchTotals add the counters over a benchmark run, with the allocations of the whole process.
type benchTotals struct {
	frames                                         int
	draws, drawRanges, entDraws, entBytes, uploads int
	uploadBytes                                    int
	mallocs0, bytes0                               uint64
}

func (b *benchTotals) frame() {
	if b.frames == 0 {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		b.mallocs0, b.bytes0 = ms.Mallocs, ms.TotalAlloc
	}
	b.frames++
	b.draws += rc.draws
	b.drawRanges += rc.drawRanges
	b.entDraws += rc.entDraws
	b.entBytes += rc.entBytes
	b.uploads += rc.uploads
	b.uploadBytes += rc.uploadBytes
}

func (b *benchTotals) mem() (mallocs, bytes uint64) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Mallocs - b.mallocs0, ms.TotalAlloc - b.bytes0
}
