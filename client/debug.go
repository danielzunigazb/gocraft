package client

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// frameStats keeps the last frame times, for the F3 screen and the benchmark.
type frameStats struct {
	times              [300]float32 // seconds
	n                  int
	updateMs, drawMs   float32 // game-thread time spent streaming columns and submitting the frame
	maxUpdate, maxDraw float32 // the worst of each over a benchmark run
	sumUpdate, sumDraw float64
	frames, slow       int // benchmark frames, and those over 12 ms
	slowUpdate         int // slow frames where streaming alone took over 4 ms
}

func (f *frameStats) add(dt float32) {
	f.times[f.n%len(f.times)] = dt
	f.n++
}

// summary is the average frame time and the slowest 1% of the recent frames, in milliseconds.
func (f *frameStats) summary() (avgMs, lowMs float32) {
	k := min(f.n, len(f.times))
	if k == 0 {
		return 0, 0
	}
	t := make([]float32, k)
	copy(t, f.times[:k])
	sort.Slice(t, func(i, j int) bool { return t[i] < t[j] })
	var sum float32
	for _, v := range t {
		sum += v
	}
	return sum / float32(k) * 1000, t[min(k-1, k*99/100)] * 1000
}

// record notes one benchmark frame's split between streaming and drawing.
func (f *frameStats) record(frameMs float32) {
	f.frames++
	f.sumUpdate += float64(f.updateMs)
	f.sumDraw += float64(f.drawMs)
	f.maxUpdate, f.maxDraw = max(f.maxUpdate, f.updateMs), max(f.maxDraw, f.drawMs)
	if frameMs > 12 {
		f.slow++
		if f.updateMs > 4 {
			f.slowUpdate++
		}
	}
}

func fps(ms float32) float32 {
	if ms <= 0 {
		return 0
	}
	return 1000 / ms
}

// drawDebug is the F3 screen: speed, what was drawn and where the time goes.
func (g *game) drawDebug(w, h int) {
	g.enableUI()
	avg, low := g.frames.summary()
	s := float32(g.scaleFor(w, h))
	st := g.stats
	pp := func(s *samples) string { return fmt.Sprintf("%.2f/%.2f", s.recent(0.5), s.recent(0.99)) }
	lines := []string{
		fmt.Sprintf("gocraft 1.8.9 (%.0f fps, avg %.1f ms, worst 1%% %.1f ms = %.0f fps)", fps(avg), avg, low, fps(low)),
		fmt.Sprintf("game thread: %.2f ms streaming, %.2f ms drawing", g.frames.updateMs, g.frames.drawMs),
		fmt.Sprintf("cpu ms p50/p99: cull %s, collect %s, solid %s, ents %s, water %s", pp(&cpuT.cull), pp(&cpuT.collect), pp(&cpuT.solid), pp(&cpuT.ents), pp(&cpuT.water)),
		fmt.Sprintf("cpu ms p50/p99: upload %s, edit %s, queue %s, mesh per column (worker) %s", pp(&cpuT.upload), pp(&cpuT.edit), pp(&cpuT.queue), pp(&cpuT.mesh)),
		fmt.Sprintf("gpu ms p50/p99: solid %s, ents %s, water %s, total %s", pp(&gpuWorld.solid), pp(&gpuWorld.ents), pp(&gpuWorld.water), pp(&gpuWorld.total)),
		fmt.Sprintf("draw calls: %d world (%d ranges), %d per-frame geometry (%.0f KB streamed), %d column uploads (%.0f KB)", rc.draws, rc.drawRanges, rc.entDraws, float64(rc.entBytes)/1024, rc.uploads, float64(rc.uploadBytes)/1024),
		fmt.Sprintf("chunks: %d drawn, %d culled, %d meshed, %d waiting, %d in memory", st.drawn, st.culled, st.loaded, st.queued, g.world.Loaded()),
		fmt.Sprintf("quads: %d (%d vertices), render distance %d", st.quads, st.quads*4, g.viewDist()),
		fmt.Sprintf("XYZ: %.3f / %.5f / %.3f", g.feet[0], g.feet[1], g.feet[2]),
		fmt.Sprintf("graphics: %s leaves, vsync %s", map[bool]string{true: "fancy", false: "fast"}[g.opts.Fancy], onOff(g.opts.VSync)),
	}
	for i, l := range lines {
		g.text(l, 2*s, (2+float32(i)*10)*s, s, [4]float32{1, 1, 1, 1}, true)
	}
}

// printBench writes the statistics of a benchmark run.
func (g *game) printBench() {
	t := append([]float32(nil), g.benchTimes...)
	if len(t) == 0 {
		fmt.Println("bench: no frames")
		return
	}
	sort.Slice(t, func(i, j int) bool { return t[i] < t[j] })
	var sum float32
	for _, v := range t {
		sum += v
	}
	at := func(p float64) float32 { return t[min(len(t)-1, int(float64(len(t))*p))] * 1000 }
	avg := sum / float32(len(t)) * 1000
	fmt.Printf("bench: %d frames over %.1f s\n", len(t), sum)
	fmt.Printf("bench: %.0f fps average (%.2f ms), median %.2f ms, 99th %.2f ms (%.0f fps), 99.9th %.2f ms, worst %.2f ms\n",
		fps(avg), avg, at(0.5), at(0.99), fps(at(0.99)), at(0.999), t[len(t)-1]*1000)
	f := &g.frames
	fmt.Printf("bench: game thread averages %.2f ms streaming (worst %.1f) and %.2f ms drawing (worst %.1f)\n",
		f.sumUpdate/float64(max(f.frames, 1)), f.maxUpdate, f.sumDraw/float64(max(f.frames, 1)), f.maxDraw)
	fmt.Printf("bench: %d frames over 12 ms (%.1f%%), %d of them with streaming over 4 ms\n", f.slow, 100*float64(f.slow)/float64(max(f.frames, 1)), f.slowUpdate)
	fmt.Println("bench: cpu, game thread, ms per frame spent asking (mesh: ms per column, on a worker)")
	for _, p := range []struct {
		name string
		s    *samples
	}{{"cull", &cpuT.cull}, {"collect", &cpuT.collect}, {"submit solid", &cpuT.solid}, {"submit ents", &cpuT.ents}, {"submit water", &cpuT.water},
		{"upload", &cpuT.upload}, {"edit remesh", &cpuT.edit}, {"queue+free", &cpuT.queue}, {"mesh", &cpuT.mesh}} {
		fmt.Println("bench:   " + p.s.line(p.name))
	}
	if gpuWorld.total.n > 0 {
		fmt.Println("bench: gpu, timestamp queries, ms per frame (each frame read a few frames late)")
		fmt.Println("bench:   " + gpuWorld.solid.line("solid"))
		fmt.Println("bench:   " + gpuWorld.ents.line("items+mobs"))
		fmt.Println("bench:   " + gpuWorld.water.line("water"))
		fmt.Println("bench:   " + gpuWorld.total.line("world total"))
	}
	if b := &g.btot; b.frames > 0 {
		n := float64(b.frames)
		mallocs, bytes := b.mem()
		fmt.Printf("bench: per frame %.0f world draw calls (%.0f ranges), %.1f per-frame-geometry draws streaming %.1f KB, %.2f column uploads (%.1f KB)\n",
			float64(b.draws)/n, float64(b.drawRanges)/n, float64(b.entDraws)/n, float64(b.entBytes)/n/1024, float64(b.uploads)/n, float64(b.uploadBytes)/n/1024)
		fmt.Printf("bench: allocations per frame %.0f (%.1f KB) over the whole process\n", float64(mallocs)/n, float64(bytes)/n/1024)
	}
	fmt.Printf("bench: last frame drew %d columns (%d culled), %d quads; render distance %d\n",
		g.stats.drawn, g.stats.culled, g.stats.quads, g.viewDist())
}

// samples collects the times of one phase, in milliseconds: a ring of the last ones for F3, and while
// a benchmark runs (timersKeep) every one, so its percentiles are exact.
type samples struct {
	ring [300]float32
	n    int       // samples taken since the last reset
	all  []float32 // every sample, while timersKeep is set
	sum  float64
	max  float32
}

// timersKeep makes samples hold every value; it is set for the length of a benchmark only, so a
// normal session does not grow.
var timersKeep bool

func (s *samples) add(ms float32) {
	s.ring[s.n%len(s.ring)] = ms
	s.n++
	s.sum += float64(ms)
	s.max = max(s.max, ms)
	if timersKeep {
		s.all = append(s.all, ms)
	}
}

// recent is the p-th (0..1) percentile of the last samples.
func (s *samples) recent(p float64) float32 {
	k := min(s.n, len(s.ring))
	if k == 0 {
		return 0
	}
	var t [len(s.ring)]float32
	copy(t[:k], s.ring[:k])
	slices.Sort(t[:k])
	return pctOf(t[:k], p)
}

// pctOf is the p-th (0..1) percentile of sorted values.
func pctOf(sorted []float32, p float64) float32 {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[min(len(sorted)-1, int(float64(len(sorted))*p))]
}

// line is the benchmark's row for a phase: every sample of the run.
func (s *samples) line(label string) string {
	if s.n == 0 || len(s.all) == 0 {
		return fmt.Sprintf("%-14s no samples", label)
	}
	t := slices.Clone(s.all)
	slices.Sort(t)
	return fmt.Sprintf("%-14s n=%-6d mean %6.3f  p50 %6.3f  p99 %6.3f  p99.9 %6.3f  max %6.3f",
		label, s.n, s.sum/float64(s.n), pctOf(t, 0.5), pctOf(t, 0.99), pctOf(t, 0.999), s.max)
}

// cpuTimers are the phases of a frame on the game thread, as wall time on the monotonic clock
// (time.Now carries one). Submitting GL calls returns before the GPU has run them, so these are the
// cost of asking, not of drawing; gpuWorld has the other half. mesh is the exception: one sample per
// column, taken on a worker.
type cpuTimers struct {
	edit    samples // meshing the columns an edit touched, and uploading them
	upload  samples // taking the finished meshes from the workers into the arenas
	queue   samples // queueing the missing columns, freeing the far ones
	cull    samples // cullSections
	collect samples // the visible columns and their quad ranges
	solid   samples // submitting the solid columns
	ents    samples // submitting items and mobs
	water   samples // submitting the water
	mesh    samples // one column on a worker
}

var cpuT cpuTimers

// since is the time from t, in milliseconds.
func since(t time.Time) float32 { return float32(time.Since(t).Nanoseconds()) / 1e6 }

// resetTimers starts the phase statistics over; keep says whether to hold every sample.
func resetTimers(keep bool) {
	cpuT = cpuTimers{}
	gpuWorld.solid, gpuWorld.ents, gpuWorld.water, gpuWorld.total = samples{}, samples{}, samples{}, samples{}
	timersKeep = keep
}

const (
	gpuMarks = 4 // stamped points of the world draw: start, solid done, items and mobs done, water done
	gpuSlots = 4 // frames whose stamps are still being waited for
)

// gpuTimer times the phases of the world draw on the GPU with timestamp queries. GL_TIME_ELAPSED
// allows one query at a time, so phases and their total cannot be taken together with it; stamps
// can follow one another freely. The results are read a few frames late, so asking never makes the
// CPU wait for the GPU. A stamp is a point on the GPU's timeline: if the CPU submits slower than the
// GPU draws, the wait lands in the phase being submitted, so read these next to cpuT.
type gpuTimer struct {
	q                         [gpuSlots][gpuMarks]uint32
	issued                    [gpuSlots]bool
	cur                       int
	open                      bool // a frame is being stamped
	next                      int  // stamps written in it
	solid, ents, water, total samples
}

// gpuWorld times the drawing of the world: columns, items and mobs, then water.
var gpuWorld gpuTimer

func (t *gpuTimer) begin() {
	if t.q[0][0] == 0 {
		for i := range t.q {
			gl.GenQueries(gpuMarks, &t.q[i][0])
		}
	}
	t.collect()
	t.open = false
	if t.issued[t.cur] { // the ring is full: the GPU is that far behind, skip this frame
		return
	}
	t.open, t.next = true, 0
	t.mark()
}

// mark stamps the next point of the frame.
func (t *gpuTimer) mark() {
	if !t.open || t.next == gpuMarks {
		return
	}
	gl.QueryCounter(t.q[t.cur][t.next], gl.TIMESTAMP)
	t.next++
}

func (t *gpuTimer) end() {
	if !t.open {
		return
	}
	for t.next < gpuMarks {
		t.mark()
	}
	t.issued[t.cur] = true
	t.cur = (t.cur + 1) % gpuSlots
	t.open = false
}

// collect reads the finished frames, oldest first. Stamps complete in order, so a frame is ready
// once its last one is.
func (t *gpuTimer) collect() {
	for i := 0; i < gpuSlots; i++ {
		k := (t.cur + i) % gpuSlots
		if !t.issued[k] {
			continue
		}
		var ready int32
		gl.GetQueryObjectiv(t.q[k][gpuMarks-1], gl.QUERY_RESULT_AVAILABLE, &ready)
		if ready == 0 {
			return
		}
		var ns [gpuMarks]uint64
		for m := range ns {
			gl.GetQueryObjectui64v(t.q[k][m], gl.QUERY_RESULT, &ns[m])
		}
		t.issued[k] = false
		ms := func(a, b int) float32 { return float32(int64(ns[b]-ns[a])) / 1e6 }
		t.solid.add(ms(0, 1))
		t.ents.add(ms(1, 2))
		t.water.add(ms(2, 3))
		t.total.add(ms(0, 3))
	}
}
