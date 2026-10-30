package client

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/jfreymuth/oggvorbis"
)

// The audio engine: OGG Vorbis files decoded to PCM (in pure Go, on a background thread), a mixer of up
// to maxVoices sounds at 44.1 kHz stereo with a pitch and a gain per voice, and an output that pipes the
// mixed PCM to the system's player (paplay, pw-cat or aplay). Nothing here touches the game's state, and
// nothing in the game ever waits on it: a sound that cannot be played right now is dropped.
//
// Why a pipe and not a sound library: it needs no cgo and no extra build weight, it talks to whatever
// PulseAudio or PipeWire is running, and the player is the same one the system already trusts.

var noSound = flag.Bool("nosound", false, "play no sound (also off for screenshots and benchmarks)")

const (
	mixRate   = 44100
	mixChunk  = 882 // 20 ms
	maxVoices = 40
)

// sample is a decoded sound: 16-bit PCM, interleaved when it has two channels.
type sample struct {
	pcm    []int16
	ch     int
	rate   int
	frames int
}

// decodeOGG reads an OGG Vorbis file into a sample.
func decodeOGG(path string) (*sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, format, err := oggvorbis.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if format.Channels < 1 || format.Channels > 2 || len(data) == 0 {
		return nil, fmt.Errorf("%s: %d channels, %d samples", path, format.Channels, len(data))
	}
	pcm := make([]int16, len(data))
	for i, v := range data {
		pcm[i] = int16(max(-1, min(1, v)) * 32767)
	}
	return &sample{pcm: pcm, ch: format.Channels, rate: format.SampleRate, frames: len(data) / format.Channels}, nil
}

// voice is one sound playing: where it is in its sample (in frames), how fast it goes (pitch and sample
// rate) and how loud in each ear.
type voice struct {
	s      *sample
	pos    float64
	step   float64
	gl, gr float32
}

type mixer struct {
	req    chan voice    // new voices, from the game thread
	wake   chan struct{} // tells the output there is something to play
	voices []voice       // only touched by the output thread
}

func newMixer() *mixer {
	return &mixer{req: make(chan voice, 256), wake: make(chan struct{}, 1)}
}

// add starts a voice; a full queue drops it.
func (m *mixer) add(v voice) {
	select {
	case m.req <- v:
		select {
		case m.wake <- struct{}{}:
		default:
		}
	default:
	}
}

// busy is whether anything is playing or waiting to.
func (m *mixer) busy() bool { return len(m.voices) > 0 || len(m.req) > 0 }

// render mixes the next len(out)/2 stereo frames of every voice into out, as signed 16-bit samples.
func (m *mixer) render(out []int16) {
	for { // new voices; past the limit the one that started earliest and has the least left gives way
		select {
		case v := <-m.req:
			if len(m.voices) >= maxVoices {
				m.voices = m.voices[1:]
			}
			m.voices = append(m.voices, v)
			continue
		default:
		}
		break
	}
	n := len(out) / 2
	for i := range out {
		out[i] = 0
	}
	if len(m.voices) == 0 {
		return
	}
	acc := make([]float32, len(out)) // allocated once per chunk: 1764 floats
	live := m.voices[:0]
	for _, v := range m.voices {
		s := v.s
		end := float64(s.frames - 1)
		for i := 0; i < n && v.pos < end; i++ {
			k := int(v.pos)
			f := float32(v.pos - float64(k))
			if s.ch == 1 {
				x := (float32(s.pcm[k])*(1-f) + float32(s.pcm[k+1])*f) / 32768
				acc[2*i] += x * v.gl
				acc[2*i+1] += x * v.gr
			} else {
				l := (float32(s.pcm[2*k])*(1-f) + float32(s.pcm[2*k+2])*f) / 32768
				r := (float32(s.pcm[2*k+1])*(1-f) + float32(s.pcm[2*k+3])*f) / 32768
				acc[2*i] += l * v.gl
				acc[2*i+1] += r * v.gr
			}
			v.pos += v.step
		}
		if v.pos < end {
			live = append(live, v)
		}
	}
	m.voices = live
	for i, x := range acc { // a soft knee past 0.8, so several loud sounds together do not clip hard
		if ax := float32(math.Abs(float64(x))); ax > 0.8 {
			x = float32(math.Copysign(float64(0.8+0.2*math.Tanh(float64((ax-0.8)/0.2))), float64(x)))
		}
		out[i] = int16(x * 32767)
	}
}

// ---- output ----

// playerCommand is the first audio player of the system that can read raw PCM on its standard input.
func playerCommand() *exec.Cmd {
	if p, err := exec.LookPath("paplay"); err == nil {
		return exec.Command(p, "--raw", "--rate=44100", "--format=s16le", "--channels=2", "--latency-msec=60",
			"--client-name=gocraft", "--stream-name=gocraft")
	}
	if p, err := exec.LookPath("pw-cat"); err == nil {
		return exec.Command(p, "--playback", "--rate", "44100", "--channels", "2", "--format", "s16", "--latency", "60ms", "-")
	}
	if p, err := exec.LookPath("aplay"); err == nil {
		return exec.Command(p, "-q", "-t", "raw", "-f", "S16_LE", "-r", "44100", "-c", "2")
	}
	return nil
}

// output feeds the mixer's PCM to a player process.
type output struct {
	cmd  *exec.Cmd
	pipe *os.File
	quit chan struct{}
	done chan struct{}
}

// startOutput starts the player and the thread that mixes into it; nil if there is no player.
func startOutput(m *mixer) (*output, error) {
	cmd := playerCommand()
	if cmd == nil {
		return nil, fmt.Errorf("no audio player found (paplay, pw-cat or aplay)")
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdin = r
	if err := cmd.Start(); err != nil {
		r.Close()
		w.Close()
		return nil, err
	}
	r.Close()
	// A small pipe keeps what is queued short, so a sound is heard soon after it is played; if the system
	// refuses (F_SETPIPE_SZ), the pacing below keeps it short anyway.
	syscall.Syscall(syscall.SYS_FCNTL, w.Fd(), 1031, 4096)
	o := &output{cmd: cmd, pipe: w, quit: make(chan struct{}), done: make(chan struct{})}
	go o.run(m)
	go cmd.Wait()
	return o, nil
}

func (o *output) run(m *mixer) {
	defer close(o.done)
	pcm := make([]int16, 2*mixChunk)
	raw := make([]byte, 4*mixChunk)
	var start time.Time
	var frames int64
	for {
		select {
		case <-o.quit:
			return
		default:
		}
		if !m.busy() { // nothing to play: wait for something, without writing silence
			start, frames = time.Time{}, 0
			select {
			case <-o.quit:
				return
			case <-m.wake:
			case <-time.After(50 * time.Millisecond):
			}
			continue
		}
		if start.IsZero() {
			start = time.Now()
		}
		m.render(pcm)
		for i, v := range pcm {
			raw[2*i], raw[2*i+1] = byte(v), byte(uint16(v)>>8)
		}
		if _, err := o.pipe.Write(raw); err != nil {
			return // the player went away
		}
		frames += mixChunk
		// Never get more than 50 ms ahead of the clock: what is queued is what is heard late.
		if ahead := time.Duration(frames)*time.Second/mixRate - time.Since(start); ahead > 50*time.Millisecond {
			time.Sleep(ahead - 50*time.Millisecond)
		}
	}
}

func (o *output) close() {
	if o == nil {
		return
	}
	close(o.quit)
	<-o.done
	o.pipe.Close()
}

// ---- the decoded sounds ----

// sampleCache decodes sound files on background threads, on first use. A sound that is not ready when it
// is asked for is skipped that once.
type sampleCache struct {
	mu      sync.Mutex
	samples map[string]*sample // by file; nil means decoding failed or is still going
	pending map[string]bool
	jobs    chan string
	path    func(name string) string // the file of a sound name; "" if there is none
}

func newSampleCache(path func(string) string) *sampleCache {
	c := &sampleCache{samples: map[string]*sample{}, pending: map[string]bool{}, jobs: make(chan string, 1024), path: path}
	for i := 0; i < 2; i++ {
		go c.worker()
	}
	return c
}

func (c *sampleCache) worker() {
	for name := range c.jobs {
		var s *sample
		if p := c.path(name); p != "" {
			var err error
			if s, err = decodeOGG(p); err != nil {
				logf("sound %s: %v", name, err)
			}
		}
		c.mu.Lock()
		c.samples[name] = s
		c.mu.Unlock()
	}
}

// get is the decoded sound, or nil if it is not ready (decoding then starts, if it had not).
func (c *sampleCache) get(name string) *sample {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.samples[name]; ok {
		return s
	}
	if !c.pending[name] {
		c.pending[name] = true
		select {
		case c.jobs <- name:
		default:
			c.pending[name] = false
		}
	}
	return nil
}

// wait blocks until every named sound has been decoded (tests and the preload use it).
func (c *sampleCache) wait(names []string, limit time.Duration) bool {
	end := time.Now().Add(limit)
	for _, n := range names {
		c.get(n)
	}
	for time.Now().Before(end) {
		c.mu.Lock()
		done := true
		for _, n := range names {
			if _, ok := c.samples[n]; !ok {
				done = false
			}
		}
		c.mu.Unlock()
		if done {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

var _ io.Writer = (*os.File)(nil)
