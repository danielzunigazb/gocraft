package client

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ---- the catalogue ----

func TestParseSoundsTakesNamesAndObjects(t *testing.T) {
	ev, err := parseSounds([]byte(`{
		"step.grass": {"category": "neutral", "sounds": ["step/grass1", "step/grass2"]},
		"mob.rabbit.hurt": {"category": "neutral", "sounds": [{"name": "mob/rabbit/hurt1", "volume": 0.5}, {"name": "mob/rabbit/hurt2", "volume": 0.5, "pitch": 1.5, "weight": 3}]},
		"empty": {"category": "block", "sounds": []}}`))
	if err != nil {
		t.Fatal(err)
	}
	if e := ev["step.grass"]; e == nil || e.category != "neutral" || len(e.sounds) != 2 || e.sounds[0].volume != 1 || e.sounds[0].pitch != 1 {
		t.Fatalf("step.grass: %+v", e)
	}
	r := ev["mob.rabbit.hurt"]
	if r == nil || r.sounds[0].volume != 0.5 || r.sounds[1].pitch != 1.5 || r.sounds[1].weight != 3 {
		t.Fatalf("rabbit: %+v", r)
	}
	if ev["empty"] != nil {
		t.Fatal("an event with no sounds is kept")
	}
	// pick follows the weights: 3 to 1.
	n := 0
	for i := 0; i < 4000; i++ {
		if r.pick().name == "mob/rabbit/hurt2" {
			n++
		}
	}
	if n < 2800 || n > 3200 {
		t.Fatalf("the weight-3 sound was picked %d times in 4000", n)
	}
}

// Every event the game asks for exists in the 1.8.9 sounds.json (the player's own folder; skipped without it).
func TestEveryEventTheGamePlaysExists(t *testing.T) {
	events, _, err := soundAssets(filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "client.jar"))
	if err != nil {
		t.Skip("no 1.8.9 assets")
	}
	need := append([]string{}, preloadEvents...)
	need = append(need, "creeper.primed", "random.eat")
	for _, k := range blockSounds {
		need = append(need, k.dig(), k.step())
	}
	for _, m := range mobSoundTable {
		need = append(need, m.say, m.hurt, m.death, m.step)
	}
	for _, ev := range need {
		if ev != "" && events[ev] == nil {
			t.Errorf("the game plays %q, which sounds.json does not have", ev)
		}
	}
}

func TestBlockSoundKinds(t *testing.T) {
	for st, want := range map[uint16]blockSound{
		1 << 4: bsStone, 4 << 4: bsStone, 2 << 4: bsGrass, 3 << 4: bsGravel, 12 << 4: bsSand, 5 << 4: bsWood, 17 << 4: bsWood,
		35 << 4: bsCloth, 20 << 4: bsGlass, 78 << 4: bsSnow, 65 << 4: bsLadder, 0: bsNone, 9 << 4: bsNone, 18 << 4: bsGrass, 88 << 4: bsSand,
	} {
		if got := soundOfBlock(st); got != want {
			t.Errorf("block %d sounds as %d, want %d", st>>4, got, want)
		}
	}
	if bsGlass.step() != "step.stone" || bsGlass.dig() != "dig.glass" || bsLadder.dig() != "dig.wood" || bsNone.dig() != "" {
		t.Fatal("glass is heard stepping as stone, a ladder breaks as wood, nothing has no sound")
	}
}

// ---- where a sound is heard ----

func TestSpatialSounds(t *testing.T) {
	// Facing +z (yaw 0): forward is +z, and the right hand is -x.
	sn, c := float32(0), float32(1)
	cases := []struct {
		name         string
		d            vec3
		wantL, wantR string // "<" the left ear hears it less than the right, ">" more, "=" the same
		dist         float32
	}{
		{"ahead", vec3{0, 0, 4}, "=", "=", 0.75},
		{"on the right", vec3{-4, 0, 0}, "<", ">", 0.75},
		{"on the left", vec3{4, 0, 0}, ">", "<", 0.75},
		{"behind", vec3{0, 0, -8}, "=", "=", 0.5},
		{"almost inside the head", vec3{0.1, 0, 0}, "=", "=", 0.99375}, // the fall-off is linear from the first block
		{"too far", vec3{0, 0, 17}, "=", "=", 0},
	}
	for _, c2 := range cases {
		l, r, d := spatial(c2.d, sn, c, 1)
		if math.Abs(float64(d-c2.dist)) > 1e-5 {
			t.Errorf("%s: distance factor %v, want %v", c2.name, d, c2.dist)
		}
		switch {
		case c2.wantL == "<" && !(l < r), c2.wantL == ">" && !(l > r), c2.wantL == "=" && math.Abs(float64(l-r)) > 1e-5:
			t.Errorf("%s: ears %v %v", c2.name, l, r)
		}
		if l > 1 || r > 1 {
			t.Errorf("%s: an ear louder than the sound itself: %v %v", c2.name, l, r)
		}
	}
	// A loud sound (volume 4, an explosion) carries 4 times as far.
	if _, _, d := spatial(vec3{0, 0, 32}, sn, c, 4); d < 0.49 || d > 0.51 {
		t.Fatalf("an explosion 32 blocks away has %v of its reach left, want 0.5", d)
	}
	// Turning the listener turns the sides: facing +x (yaw -90 degrees: forward is (-sin, 0, cos) = (1, 0, 0)),
	// the south (+z) is on the right hand, as when facing east in the game.
	yaw := -math.Pi / 2
	l, r, _ := spatial(vec3{0, 0, 4}, float32(math.Sin(yaw)), float32(math.Cos(yaw)), 1)
	if !(r > l) {
		t.Fatalf("facing east, a sound to the south should be on the right: %v %v", l, r)
	}
}

// ---- the mixer ----

func sine(frames int, ch int) *sample {
	s := &sample{pcm: make([]int16, frames*ch), ch: ch, rate: mixRate, frames: frames}
	for i := 0; i < frames; i++ {
		v := int16(16000 * math.Sin(2*math.Pi*441*float64(i)/mixRate))
		for c := 0; c < ch; c++ {
			s.pcm[i*ch+c] = v
		}
	}
	return s
}

func peak(out []int16, side int) int {
	p := 0
	for i := side; i < len(out); i += 2 {
		v := int(out[i])
		if v < 0 {
			v = -v
		}
		p = max(p, v)
	}
	return p
}

func TestMixerPlaysPansAndEnds(t *testing.T) {
	m := newMixer()
	out := make([]int16, 2*mixChunk)
	m.render(out)
	if peak(out, 0) != 0 || m.busy() {
		t.Fatal("silence expected with nothing playing")
	}
	m.add(voice{s: sine(2000, 1), step: 1, gl: 0.5, gr: 0.25})
	m.render(out)
	l, r := peak(out, 0), peak(out, 1)
	if l < 7500 || l > 8100 || r < 3700 || r > 4100 { // 16000 * 0.5 and * 0.25
		t.Fatalf("peaks %d %d, want about 8000 and 4000", l, r)
	}
	m.render(out) // 882 frames a chunk: the 2000 of the sample end during the third, which still has its tail
	m.render(out)
	if m.busy() || peak(out, 0) == 0 {
		t.Fatalf("the voice should have ended in the third chunk with its tail (busy %v, peak %d)", m.busy(), peak(out, 0))
	}
	m.render(out)
	if peak(out, 0) != 0 {
		t.Fatal("sound after the voice ended")
	}
	// Twice the pitch is twice the speed: it ends in half the frames.
	m.add(voice{s: sine(3000, 1), step: 2, gl: 1, gr: 1})
	m.render(out)
	if !m.busy() {
		t.Fatal("ended too soon")
	}
	m.render(out) // 3000 frames at step 2 = 1500 steps: gone during the second chunk
	if m.busy() {
		t.Fatal("a pitch of 2 should have ended it within 2 chunks")
	}
	// Past the limit the oldest gives way.
	for i := 0; i < maxVoices+10; i++ {
		m.add(voice{s: sine(50000, 2), step: 1, gl: 0.01, gr: 0.01})
	}
	m.render(out)
	if len(m.voices) != maxVoices {
		t.Fatalf("%d voices, limit %d", len(m.voices), maxVoices)
	}
	// Many loud voices do not clip hard: the output stays inside 16 bits.
	loud := newMixer()
	for i := 0; i < 20; i++ {
		loud.add(voice{s: sine(5000, 1), step: 1, gl: 1, gr: 1})
	}
	loud.render(out)
	if p := peak(out, 0); p > 32767 || p < 24000 {
		t.Fatalf("20 loud voices peak at %d", p)
	}
}

// ---- the system, with its output replaced by a recorder ----

func recordingSound(vols map[string]float32) (*soundSystem, *[]playedSound) {
	ev, _ := parseSounds([]byte(`{
		"step.grass": {"category": "neutral", "sounds": ["step/grass1"]},
		"dig.stone": {"category": "block", "sounds": ["dig/stone1"]},
		"gui.button.press": {"category": "master", "sounds": ["random/click"]},
		"random.explode": {"category": "block", "sounds": ["random/explode1"]},
		"mob.zombie.say": {"category": "hostile", "sounds": ["mob/zombie/say1"]},
		"mob.zombie.hurt": {"category": "hostile", "sounds": ["mob/zombie/hurt1"]},
		"mob.zombie.death": {"category": "hostile", "sounds": ["mob/zombie/death"]},
		"mob.zombie.step": {"category": "hostile", "sounds": ["mob/zombie/step1"]},
		"creeper.primed": {"category": "hostile", "sounds": ["mob/creeper/primed"]},
		"mob.creeper.say": {"category": "hostile", "sounds": ["mob/creeper/say1"]},
		"mob.creeper.death": {"category": "hostile", "sounds": ["mob/creeper/death"]}}`))
	var got []playedSound
	s := &soundSystem{events: ev, cache: newSampleCache(func(string) string { return "" }), mix: newMixer(),
		vol: func(c string) float32 {
			if v, ok := vols[c]; ok {
				return v
			}
			return 1
		}}
	s.record = func(p playedSound) { got = append(got, p) }
	return s, &got
}

func TestSoundGainsFollowTheCategories(t *testing.T) {
	s, got := recordingSound(map[string]float32{"master": 0.5, "block": 0.5, "neutral": 0})
	s.listen(vec3{}, 0)
	s.play("dig.stone", vec3{0, 0, 0}, 1, 0.8)
	if len(*got) != 1 || math.Abs(float64((*got)[0].gain-0.25)) > 1e-5 || (*got)[0].pitch != 0.8 {
		t.Fatalf("%+v: master 0.5 x block 0.5 should be a gain of 0.25", *got)
	}
	s.play("step.grass", vec3{}, 0.15, 1)
	if len(*got) != 1 {
		t.Fatal("a category at 0 still played")
	}
	// A sound of the master category answers to the master volume only, and is not placed.
	s.playUI("gui.button.press", 1, 1)
	if n := len(*got); n != 2 || !(*got)[1].relative || math.Abs(float64((*got)[1].gain-0.5)) > 1e-5 {
		t.Fatalf("ui: %+v", *got)
	}
	// Beyond reach: nothing. At 8 blocks: half.
	s.play("dig.stone", vec3{0, 0, 17}, 1, 1)
	if len(*got) != 2 {
		t.Fatal("a sound 17 blocks away played")
	}
	s.play("dig.stone", vec3{0, 0, 8}, 1, 1)
	if g := (*got)[2].gain; math.Abs(float64(g-0.125)) > 1e-5 {
		t.Fatalf("at 8 blocks the gain is %v, want 0.25 x 0.5", g)
	}
	s.play("no.such.event", vec3{}, 1, 1) // an unknown event is silently nothing
	// The pitch is kept within the game's 0.5 .. 2.
	s.play("random.explode", vec3{}, 4, 9)
	if p := (*got)[3].pitch; p != 2 {
		t.Fatalf("pitch %v", p)
	}
	var nilSys *soundSystem // the game with sound off calls these on nil
	nilSys.play("dig.stone", vec3{}, 1, 1)
	nilSys.playUI("gui.button.press", 1, 1)
	nilSys.listen(vec3{}, 0)
	nilSys.close()
}

func soundGame(t *testing.T) (*game, *[]playedSound) {
	g := testGame(t)
	s, got := recordingSound(nil)
	g.snd = s
	g.pos = vec3{0.5, 5.62, 0.5}
	g.feet = vec3{0.5, 4, 0.5}
	g.prevFeet = g.feet
	g.onGround = true
	s.listen(g.pos, g.yaw)
	return g, got
}

func count(got []playedSound, event string) int {
	n := 0
	for _, p := range got {
		if p.event == event {
			n++
		}
	}
	return n
}

// Walking makes a step every 1 / 0.6 blocks, from the block under the feet, at volume 0.15.
func TestFootsteps(t *testing.T) {
	g, got := soundGame(t)
	for i := 0; i < 100; i++ { // 10 blocks at a walk (0.1 a tick), the head going with the feet
		g.prevFeet = g.feet
		g.feet[0] += 0.1
		g.pos = g.feet.add(vec3{0, 1.62, 0})
		g.tickSound()
	}
	if n := count(*got, "step.grass"); n < 5 || n > 7 {
		t.Fatalf("%d steps over 10 blocks, want about 6", n)
	}
	// Played at the feet, 1.62 blocks under the ears: Entity.playStepSound's 0.15, less the linear fall-off over that distance.
	want := 0.15 * (1 - 1.62/16)
	for _, p := range *got {
		if p.event == "step.grass" && math.Abs(float64(p.gain)-want) > 0.002 {
			t.Fatalf("a step at gain %v, want %v", p.gain, want)
		}
	}
	// Flying makes none; in the air makes none until there is ground.
	*got = nil
	g.flying = true
	for i := 0; i < 50; i++ {
		g.prevFeet = g.feet
		g.feet[0] += 0.1
		g.tickSound()
	}
	if len(*got) != 0 {
		t.Fatalf("%d steps while flying", len(*got))
	}
}

func TestBlocksBrokenAndPlacedSound(t *testing.T) {
	g, got := soundGame(t)
	g.setBlock(4, 4, 4, 1<<4) // placed into air
	g.setBlock(4, 4, 4, 0)    // broken
	if count(*got, "dig.stone") != 2 {
		t.Fatalf("%d dig sounds, want 2 (placing and breaking)", count(*got, "dig.stone"))
	}
	for _, p := range *got {
		if p.pitch != 0.8 || p.gain < 0.6 || p.gain > 1 { // volume (1+1)/2 = 1, less the fall-off over a few blocks
			t.Fatalf("%+v: dig sounds are at pitch 0.8 and near full volume", p)
		}
	}
	*got = nil
	g.setBlock(4, 4, 4, 1<<4)
	g.setBlock(4, 4, 4, 2<<4) // a block turned into another is neither broken nor placed
	if len(*got) != 1 {
		t.Fatalf("%d sounds for a replaced block", len(*got))
	}
}

func TestMobsHurtDieAndSpeak(t *testing.T) {
	g, got := soundGame(t)
	g.feet = vec3{100, 4, 100} // out of the way of its attacks
	z := g.newMob(kinds["Zombie"], vec3{3, 4, 0}, 0)
	g.mobs = []*mob{z}
	g.mobSounds(z) // the first sight records its health
	z.health -= 3
	g.mobSounds(z)
	if count(*got, "mob.zombie.hurt") != 1 {
		t.Fatalf("hurt sounds: %d", count(*got, "mob.zombie.hurt"))
	}
	g.mobSounds(z) // no new blow: no new sound
	if count(*got, "mob.zombie.hurt") != 1 {
		t.Fatal("a second hurt sound without a second blow")
	}
	z.health = 0
	g.mobSounds(z)
	g.mobSounds(z)
	if count(*got, "mob.zombie.death") != 1 || count(*got, "mob.zombie.hurt") != 1 {
		t.Fatalf("death: %d, hurt: %d", count(*got, "mob.zombie.death"), count(*got, "mob.zombie.hurt"))
	}
	// A living one speaks now and then: about once in 100 ticks.
	*got = nil
	z2 := g.newMob(kinds["Zombie"], vec3{3, 4, 0}, 0)
	g.mobs = append(g.mobs, z2)
	for i := 0; i < 10000; i++ {
		g.mobSounds(z2)
	}
	if n := count(*got, "mob.zombie.say"); n < 50 || n > 150 {
		t.Fatalf("a zombie spoke %d times in 10000 ticks, want about 100", n)
	}
	// A creeper that lights its fuse hisses once.
	*got = nil
	c := g.newMob(kinds["Creeper"], vec3{3, 4, 0}, 0)
	g.mobSounds(c)
	c.fuse = 5
	g.mobSounds(c)
	c.fuse = 6
	g.mobSounds(c)
	if count(*got, "creeper.primed") != 1 {
		t.Fatalf("primed %d times", count(*got, "creeper.primed"))
	}
	// Too far to hear: nothing, but its health is still followed.
	*got = nil
	far := g.newMob(kinds["Zombie"], vec3{300, 4, 0}, 0)
	far.health = 5
	g.mobSounds(far)
	far.health = 1
	g.mobSounds(far)
	if len(*got) != 0 {
		t.Fatal("a mob 300 blocks away was heard")
	}
}

// ---- real files and the real output (the player's own 1.8.9 folder; skipped without it) ----

func realJar() string { return filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "client.jar") }

func TestDecodesRealSounds(t *testing.T) {
	_, file, err := soundAssets(realJar())
	if err != nil {
		t.Skip("no 1.8.9 assets")
	}
	for _, name := range []string{"step/grass1", "dig/stone1", "random/explode1", "mob/zombie/say1"} {
		p := file(name)
		if p == "" {
			t.Fatalf("no file for %s", name)
		}
		s, err := decodeOGG(p)
		if err != nil {
			t.Fatal(err)
		}
		if s.frames < 1000 || s.rate < 8000 || s.ch < 1 || s.ch > 2 || peak(s.pcm, 0) == 0 {
			t.Fatalf("%s: %d frames at %d Hz, %d channels, peak %d", name, s.frames, s.rate, s.ch, peak(s.pcm, 0))
		}
	}
	c := newSampleCache(file)
	if !c.wait([]string{"step/grass1", "step/grass2"}, 5*time.Second) {
		t.Fatal("the background decoding did not finish")
	}
	if c.get("step/grass1") == nil {
		t.Fatal("not cached")
	}
}

// pulseStream finds our player's stream in PulseAudio by its process id: the stream's index and the latency
// PulseAudio reports for it (what is queued in its buffer plus in the sink).
func pulseStream(pactl string, pid int) (index int, latency time.Duration, ok bool) {
	out, err := exec.Command(pactl, "list", "sink-inputs").Output()
	if err != nil {
		return 0, 0, false
	}
	for _, block := range strings.Split(string(out), "Sink Input #")[1:] {
		if !strings.Contains(block, fmt.Sprintf("application.process.id = %q", fmt.Sprint(pid))) {
			continue
		}
		fmt.Sscanf(block, "%d", &index)
		for _, line := range strings.Split(block, "\n") {
			var us int
			if strings.Contains(line, "Buffer Latency:") || strings.Contains(line, "Sink Latency:") {
				if i := strings.Index(line, ":"); i >= 0 {
					fmt.Sscanf(strings.TrimSpace(line[i+1:]), "%d", &us)
					latency += time.Duration(us) * time.Microsecond
				}
			}
		}
		return index, latency, true
	}
	return 0, 0, false
}

// The whole path to the speakers, with the real player and the real PulseAudio (PipeWire here): the mixed
// sound is piped to the player; its own stream is found in PulseAudio and recorded on its own, so any other
// audio on the machine (a game that is open, music) does not count; and PulseAudio's own report of the
// stream's latency says nothing piles up. Quiet, and skipped without the tools or in -short.
func TestRealOutputIsNotSilent(t *testing.T) {
	if testing.Short() {
		t.Skip("plays a sound on the machine")
	}
	events, file, err := soundAssets(realJar())
	if err != nil {
		t.Skip("no 1.8.9 assets")
	}
	parec, err1 := exec.LookPath("parec")
	pactl, err2 := exec.LookPath("pactl")
	if err1 != nil || err2 != nil || !audioAvailable() {
		t.Skip("no PulseAudio tools")
	}
	sinkB, err := exec.Command(pactl, "get-default-sink").Output()
	if err != nil {
		t.Skip("no default sink")
	}
	monitor := strings.TrimSpace(string(sinkB)) + ".monitor"

	s := &soundSystem{events: events, cache: newSampleCache(file), mix: newMixer(), vol: func(string) float32 { return 1 }}
	if s.out, err = startOutput(s.mix); err != nil {
		t.Skip("no output: " + err.Error())
	}
	defer s.close()
	s.cache.wait([]string{"step/grass1", "step/grass2", "step/grass3", "step/grass4", "step/grass5", "step/grass6", "random/explode1"}, 5*time.Second)

	// The game thread never waits on the audio: a thousand requests in a row take no time at all.
	t0 := time.Now()
	for i := 0; i < 1000; i++ {
		s.mix.add(voice{s: sine(10, 1), step: 1})
	}
	if d := time.Since(t0); d > 20*time.Millisecond {
		t.Fatalf("1000 voices queued in %v: the game thread waited", d)
	}
	time.Sleep(200 * time.Millisecond)

	// Find our stream (it exists once the player has been given data) and record just that one.
	pid := s.out.sink.pid
	s.playUI("step.grass", 0.3, 1)
	var idx int
	for i := 0; i < 40; i++ {
		var ok bool
		if idx, _, ok = pulseStream(pactl, pid); ok {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if idx == 0 {
		t.Fatalf("the player's stream (pid %d) never appeared in PulseAudio", pid)
	}
	rec := filepath.Join(t.TempDir(), "rec.raw")
	cmd := exec.Command(parec, "-d", monitor, fmt.Sprintf("--monitor-stream=%d", idx), "--format=s16le", "--rate=44100",
		"--channels=2", "--latency-msec=20", "--file-format=raw", rec)
	if err := cmd.Start(); err != nil {
		t.Skip("cannot record: " + err.Error())
	}
	defer cmd.Process.Kill()
	time.Sleep(300 * time.Millisecond)

	for i := 0; i < 4; i++ {
		s.playUI("step.grass", 0.3, 1) // quiet, but plenty for a recording
		time.Sleep(450 * time.Millisecond)
	}
	// A long sound, to read the stream's latency while it plays: PulseAudio's buffer plus the sink's.
	s.playUI("random.explode", 0.08, 1)
	var worst time.Duration
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, lat, ok := pulseStream(pactl, pid); ok {
			worst = max(worst, lat)
		}
	}
	select {
	case <-s.out.done:
		t.Fatal("the output thread ended: the player went away")
	default:
	}
	cmd.Process.Signal(syscall.SIGTERM)
	cmd.Wait()

	raw, err := os.ReadFile(rec)
	if err != nil || len(raw) < 4*44100/4 {
		t.Fatalf("recorded %d bytes of our own stream: %v", len(raw), err)
	}
	var maxv int
	var sum float64
	for i := 0; i+1 < len(raw); i += 2 {
		v := int(int16(uint16(raw[i]) | uint16(raw[i+1])<<8))
		sum += float64(v * v)
		maxv = max(maxv, max(v, -v))
	}
	t.Logf("our stream (#%d on %s): %d bytes, peak %d, rms %.1f; PulseAudio reports a latency of at most %v", idx, monitor, len(raw), maxv, math.Sqrt(sum/float64(len(raw)/2)), worst)
	if maxv < 300 {
		t.Fatalf("our stream is silent (peak %d): the sound did not reach PulseAudio", maxv)
	}
	if worst > 300*time.Millisecond {
		t.Fatalf("latency %v: what is queued is heard late", worst)
	}
}

// writeWAV writes 16-bit stereo PCM as a WAV file.
func writeWAV(t *testing.T, path string, pcm []int16) {
	t.Helper()
	b := make([]byte, 44+2*len(pcm))
	le := func(o int, v uint32, n int) {
		for i := 0; i < n; i++ {
			b[o+i] = byte(v >> (8 * i))
		}
	}
	copy(b, "RIFF")
	le(4, uint32(36+2*len(pcm)), 4)
	copy(b[8:], "WAVEfmt ")
	le(16, 16, 4)
	le(20, 1, 2)
	le(22, 2, 2)
	le(24, mixRate, 4)
	le(28, mixRate*4, 4)
	le(32, 4, 2)
	le(34, 16, 2)
	copy(b[36:], "data")
	le(40, uint32(2*len(pcm)), 4)
	for i, v := range pcm {
		le(44+2*i, uint32(uint16(v)), 2)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// With no sound device at all: the mix of a real explosion heard from the right goes to a WAV file, which is
// read back and measured: it has energy, it is louder in the right ear, and it dies away.
func TestMixToWAVHasEnergyAndPan(t *testing.T) {
	_, file, err := soundAssets(realJar())
	if err != nil {
		t.Skip("no 1.8.9 assets")
	}
	cache := newSampleCache(file)
	if !cache.wait([]string{"random/explode1"}, 5*time.Second) {
		t.Fatal("not decoded")
	}
	smp := cache.get("random/explode1")
	// Heard from 6 blocks away on the listener's right, at volume 4 (an explosion).
	pl, pr, dist := spatial(vec3{-6, 0, 0}, 0, 1, 4)
	m := newMixer()
	m.add(voice{s: smp, step: float64(smp.rate) / mixRate, gl: 4 * pl * dist * 0.25, gr: 4 * pr * dist * 0.25})
	var all []int16
	chunk := make([]int16, 2*mixChunk)
	for i := 0; i < 200 && (i == 0 || m.busy()); i++ {
		m.render(chunk)
		all = append(all, chunk...)
	}
	path := filepath.Join(t.TempDir(), "explosion.wav")
	writeWAV(t, path, all)
	raw, err := os.ReadFile(path)
	if err != nil || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		t.Fatal("not a WAV")
	}
	var el, er float64
	var last int
	for i := 44; i+3 < len(raw); i += 4 {
		l := float64(int16(uint16(raw[i]) | uint16(raw[i+1])<<8))
		r := float64(int16(uint16(raw[i+2]) | uint16(raw[i+3])<<8))
		el += l * l
		er += r * r
		if l != 0 || r != 0 {
			last = i
		}
	}
	t.Logf("%.2f s: left energy %.3g, right energy %.3g (%.1f dB right over left)", float64(len(all)/2)/mixRate, el, er, 10*math.Log10(er/el))
	if er < 1e8 || !(er > el*1.5) {
		t.Fatalf("energy left %.3g right %.3g: the explosion should be clearly louder on the right", el, er)
	}
	if last < 44+4*mixRate/10 {
		t.Fatal("the explosion is over within a tenth of a second")
	}
}

func TestSoundOffWhereItMustBe(t *testing.T) {
	if !soundWanted(Options{}, false, "") {
		t.Fatal("a normal run should have sound")
	}
	for name, off := range map[string]bool{
		"a screenshot": !soundWanted(Options{Screenshot: "x.png"}, false, ""),
		"a benchmark":  !soundWanted(Options{Bench: 5}, false, ""),
		"-nosound":     !soundWanted(Options{}, true, ""),
		"the env var":  !soundWanted(Options{}, false, "1"),
	} {
		if !off {
			t.Errorf("%s still opens the audio output", name)
		}
	}
	// A game built the way the tests build it has no sound system: every sound call is a no-op on nil.
	g := testGame(t)
	if g.snd != nil {
		t.Fatal("a test game opened the audio output")
	}
	g.blockSounds(1, 4, 1, 0, 1<<4)
	g.mobSounds(g.newMob(kinds["Zombie"], vec3{3, 4, 0}, 0))
	g.tickSound()
	g.bowSound(1)
	g.uiSound()
}
