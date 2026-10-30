package client

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The sound catalogue and the way the game asks for a sound. The names and groupings are those of the
// 1.8.9 assets (sounds.json): an event such as "step.grass" holds several files, one is picked at random,
// and the event says which volume category it belongs to. The sound files themselves stay in the player's
// own 1.8.9 folder and are read from there at run time; nothing of them is copied.

type soundEntry struct {
	name          string // under minecraft/sounds/, without .ogg
	volume, pitch float32
	weight        int
}

type soundEvent struct {
	category string
	sounds   []soundEntry
}

// parseSounds reads sounds.json: every event, with entries that are either a name or an object.
func parseSounds(b []byte) (map[string]*soundEvent, error) {
	var raw map[string]struct {
		Category string            `json:"category"`
		Sounds   []json.RawMessage `json:"sounds"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]*soundEvent, len(raw))
	for ev, r := range raw {
		e := &soundEvent{category: r.Category}
		for _, rs := range r.Sounds {
			var name string
			if json.Unmarshal(rs, &name) == nil {
				e.sounds = append(e.sounds, soundEntry{name: name, volume: 1, pitch: 1, weight: 1})
				continue
			}
			var o struct {
				Name   string   `json:"name"`
				Volume *float32 `json:"volume"`
				Pitch  *float32 `json:"pitch"`
				Weight *int     `json:"weight"`
			}
			if json.Unmarshal(rs, &o) != nil || o.Name == "" {
				continue
			}
			s := soundEntry{name: o.Name, volume: 1, pitch: 1, weight: 1}
			if o.Volume != nil {
				s.volume = *o.Volume
			}
			if o.Pitch != nil {
				s.pitch = *o.Pitch
			}
			if o.Weight != nil && *o.Weight > 0 {
				s.weight = *o.Weight
			}
			e.sounds = append(e.sounds, s)
		}
		if len(e.sounds) > 0 {
			out[ev] = e
		}
	}
	return out, nil
}

// soundAssets finds the 1.8.9 assets next to the client jar and returns the file of each sound.
func soundAssets(jarPath string) (events map[string]*soundEvent, file func(name string) string, err error) {
	dir := filepath.Join(filepath.Dir(jarPath), "assets")
	idxFiles, _ := filepath.Glob(filepath.Join(dir, "indexes", "*.json"))
	if len(idxFiles) == 0 {
		return nil, nil, os.ErrNotExist
	}
	idxPath := idxFiles[0]
	for _, f := range idxFiles {
		if filepath.Base(f) == "1.8.json" {
			idxPath = f
		}
	}
	b, err := os.ReadFile(idxPath)
	if err != nil {
		return nil, nil, err
	}
	var idx struct {
		Objects map[string]struct {
			Hash string `json:"hash"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, nil, err
	}
	object := func(key string) string {
		o, ok := idx.Objects[key]
		if !ok || len(o.Hash) < 3 {
			return ""
		}
		return filepath.Join(dir, "objects", o.Hash[:2], o.Hash)
	}
	sj, err := os.ReadFile(object("minecraft/sounds.json"))
	if err != nil {
		return nil, nil, err
	}
	events, err = parseSounds(sj)
	if err != nil {
		return nil, nil, err
	}
	return events, func(name string) string { return object("minecraft/sounds/" + name + ".ogg") }, nil
}

// soundSystem is what the game plays sounds through. Every method is safe on a nil *soundSystem, which
// is how the game runs with sound off (screenshots, benchmarks, tests, -nosound).
type soundSystem struct {
	events map[string]*soundEvent
	cache  *sampleCache
	mix    *mixer
	out    *output
	vol    func(category string) float32 // 0..1 from the options; "master" is the master volume

	// the listener: the player's head and which way it faces
	mu     sync.Mutex
	ear    vec3
	sin, c float32

	// test hook: every sound that would have played
	record func(playedSound)
}

type playedSound struct {
	event        string
	at           vec3
	gain, pitch  float32
	relative     bool
	left, right  float32
	file         string
	distanceGain float32
}

// soundWanted is whether a run opens the audio output at all: not for screenshots or benchmarks, not with
// -nosound or GOCRAFT_NO_AUDIO set. (Tests never go through Run, so they never open it either.)
func soundWanted(opt Options, noFlag bool, env string) bool {
	return opt.Screenshot == "" && opt.Bench == 0 && !noFlag && env == ""
}

// newSoundSystem opens the assets and the output; nil (with a log line) if either is missing.
func newSoundSystem(jarPath string, vol func(string) float32) *soundSystem {
	events, file, err := soundAssets(jarPath)
	if err != nil {
		logf("no sound: the 1.8.9 assets are not next to %s (%v)", jarPath, err)
		return nil
	}
	s := &soundSystem{events: events, cache: newSampleCache(file), mix: newMixer(), vol: vol}
	if s.out, err = startOutput(s.mix); err != nil {
		logf("no sound: %v", err)
		return nil
	}
	go s.preload()
	return s
}

// preloadEvents are decoded in the background at start, so the first footstep is not missed.
var preloadEvents = []string{"step.grass", "step.stone", "step.gravel", "step.sand", "step.wood", "step.cloth", "step.snow", "step.ladder",
	"dig.grass", "dig.stone", "dig.gravel", "dig.sand", "dig.wood", "dig.cloth", "dig.snow", "dig.glass",
	"gui.button.press", "random.bow", "random.bowhit", "random.explode", "random.eat", "random.burp", "random.pop", "random.break",
	"game.player.hurt", "game.player.die"}

func (s *soundSystem) preload() {
	var names []string
	for _, ev := range preloadEvents {
		if e := s.events[ev]; e != nil {
			for _, en := range e.sounds {
				names = append(names, en.name)
			}
		}
	}
	for _, k := range mobSoundTable {
		for _, ev := range []string{k.say, k.hurt, k.death, k.step} {
			if e := s.events[ev]; e != nil && ev != "" {
				for _, en := range e.sounds {
					names = append(names, en.name)
				}
			}
		}
	}
	for _, n := range names {
		s.cache.get(n)
	}
}

func (s *soundSystem) close() {
	if s == nil {
		return
	}
	s.out.close()
}

// listen tells the system where the player's head is and which way it faces (yaw in radians).
func (s *soundSystem) listen(ear vec3, yaw float32) {
	if s == nil {
		return
	}
	sn, c := math.Sincos(float64(yaw))
	s.mu.Lock()
	s.ear, s.sin, s.c = ear, float32(sn), float32(c)
	s.mu.Unlock()
}

// play plays an event at a place: volume and pitch are the caller's (1.8's playSound arguments), and the
// rest comes from the event: which file, its own volume and pitch, and the category's volume.
func (s *soundSystem) play(event string, at vec3, vol, pitch float32) {
	s.start(event, at, vol, pitch, false)
}

// playUI plays an event inside the head, with no place: the sounds of menus.
func (s *soundSystem) playUI(event string, vol, pitch float32) {
	s.start(event, vec3{}, vol, pitch, true)
}

func (s *soundSystem) start(event string, at vec3, vol, pitch float32, relative bool) {
	if s == nil {
		return
	}
	ev := s.events[event]
	if ev == nil {
		return
	}
	en := ev.pick()
	gain := vol * en.volume * s.vol("master")
	if ev.category != "master" {
		gain *= s.vol(ev.category)
	}
	if gain <= 0 {
		return
	}
	pitch = max(0.5, min(2, pitch*en.pitch))
	l, r := gain, gain
	dg := float32(1)
	if !relative {
		s.mu.Lock()
		ear, sn, c := s.ear, s.sin, s.c
		s.mu.Unlock()
		var pl, pr float32
		pl, pr, dg = spatial(at.sub(ear), sn, c, vol)
		if dg <= 0 {
			return
		}
		l, r = gain*pl*dg, gain*pr*dg
	}
	file := en.name
	smp := s.cache.get(file)
	if s.record != nil {
		s.record(playedSound{event: event, at: at, gain: gain * dg, pitch: pitch, relative: relative, left: l, right: r, file: file, distanceGain: dg})
	}
	if smp == nil {
		return // still decoding: this one is skipped
	}
	s.mix.add(voice{s: smp, step: float64(smp.rate) / mixRate * float64(pitch), gl: l, gr: r})
}

// pick chooses one of the event's files, by weight.
func (e *soundEvent) pick() soundEntry {
	total := 0
	for _, s := range e.sounds {
		total += s.weight
	}
	n := rng.Intn(total)
	for _, s := range e.sounds {
		if n -= s.weight; n < 0 {
			return s
		}
	}
	return e.sounds[0]
}

// spatial is how a sound at offset d from the listener (facing yaw: sin and cos given) sounds: the share
// of each ear (1 is the sound's own gain) and the distance factor, which is applied after. The sound fades linearly to nothing at 16 blocks (16 x the volume
// if that is above 1), and is panned by the angle to the side the listener's right hand is on.
func spatial(d vec3, sn, c float32, vol float32) (l, r, dist float32) {
	reach := float32(16)
	if vol > 1 {
		reach *= vol
	}
	dd := float32(math.Sqrt(float64(d.dot(d))))
	dist = max(0, 1-dd/reach)
	// The game's yaw 0 faces +z and turns towards -x: forward is (-sin, 0, cos), and the right hand (-cos, 0, -sin).
	right := float32(float64(-c)*d[0] - float64(sn)*d[2])
	pan := float32(0)
	if h := float32(math.Hypot(float64(d[0]), float64(d[2]))); h > 0.5 {
		pan = right / h // -1 all left .. 1 all right
	}
	// The ear on the far side hears it down to 30%; the near one is never louder than the centre.
	l, r = 1-max(0, pan)*0.7, 1-max(0, -pan)*0.7
	return l, r, dist
}

// ---- the options' volumes ----

var soundCategories = []string{"master", "music", "record", "weather", "block", "hostile", "neutral", "player", "ambient"}

// soundVolume is the volume of a category, 0..1; 1 for one that is not set.
func (o *options) soundVolume(category string) float32 {
	if v, ok := o.SoundVolume[category]; ok {
		return v
	}
	return 1
}

func soundCategoryLabel(c string) string {
	switch c {
	case "master":
		return "Master Volume"
	case "record":
		return "Jukebox/Noteblocks"
	}
	return strings.ToUpper(c[:1]) + c[1:]
}
