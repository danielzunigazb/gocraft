package client

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gocraft/world"
)

// Saving. The worlds the game opens belong to the player's own 1.8.9 folder, so changes are never
// written into them: each world gets an overlay folder with the columns that were edited and the
// player's state, applied on top of the original when the world is opened again.

const overlayMagic = "GCH1"

// overlayRoot is where overlays live: $XDG_DATA_HOME/gocraft/overlay, else ~/.local/share/gocraft/overlay.
func overlayRoot() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "gocraft", "overlay")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "gocraft", "overlay")
}

type overlay struct{ dir string }

func overlayFor(worldDir string) overlay {
	return overlay{filepath.Join(overlayRoot(), filepath.Base(worldDir))}
}

func (o overlay) chunkPath(cp world.ChunkPos) string {
	return filepath.Join(o.dir, fmt.Sprintf("c.%d.%d.bin", cp.X, cp.Z))
}

// writeAtomic writes through a temporary file, so a crash never leaves a half-written column.
func writeAtomic(path string, write func(w io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(f)
	err = write(bw)
	if err == nil {
		err = bw.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func (o overlay) saveChunk(cp world.ChunkPos, c *world.Chunk) error {
	return writeAtomic(o.chunkPath(cp), func(w io.Writer) error {
		if _, err := io.WriteString(w, overlayMagic); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, c.Blocks[:]); err != nil {
			return err
		}
		for _, opt := range [][]uint8{c.Light, c.Biomes} {
			present := byte(0)
			if opt != nil {
				present = 1
			}
			if _, err := w.Write([]byte{present}); err != nil {
				return err
			}
			if opt != nil {
				if _, err := w.Write(opt); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// loadChunk returns the edited column, or nil if the world has none saved there.
func (o overlay) loadChunk(cp world.ChunkPos) (*world.Chunk, error) {
	f, err := os.Open(o.chunkPath(cp))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	magic := make([]byte, len(overlayMagic))
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != overlayMagic {
		return nil, fmt.Errorf("%s: not a gocraft column", o.chunkPath(cp))
	}
	c := &world.Chunk{}
	if err := binary.Read(r, binary.LittleEndian, c.Blocks[:]); err != nil {
		return nil, err
	}
	for i, size := range []int{65536, 256} {
		present, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if present == 0 {
			continue
		}
		buf := make([]uint8, size)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		if i == 0 {
			c.Light = buf
		} else {
			c.Biomes = buf
		}
	}
	return c, nil
}

// playerState is what is kept of the player between sessions.
type playerState struct {
	Pos      [3]float64    `json:"pos"` // feet: doubles; older files hold the same numbers rounded to a float
	Yaw      float32       `json:"yaw"`
	Pitch    float32       `json:"pitch"`
	Hotbar   [9]uint16     `json:"hotbar,omitempty"` // saves from before items: block states and counts
	Counts   [9]int8       `json:"counts,omitempty"`
	Inv      [36]stackJSON `json:"inv"`
	Armor    [4]stackJSON  `json:"armor"`
	Selected int           `json:"selected"`
	Health   float32       `json:"health"`
	Food     int           `json:"food"`
	Flying   bool          `json:"flying"`
	Time     int64         `json:"time"`
	Dim      int           `json:"dim,omitempty"` // 0 Overworld, -1 Nether, 1 End
}

type stackJSON struct {
	ID  uint16 `json:"id"`
	Dmg uint16 `json:"dmg"`
	N   int8   `json:"n"`
}

func toJSON(s slot) stackJSON { return stackJSON{s.ID, s.Dmg, s.N} }

// inventory is the saved inventory, reading the older format with only a hotbar of blocks too.
func (p playerState) inventory() (inv [36]slot, armor [4]slot) {
	for i, s := range p.Inv {
		inv[i] = slot{s.ID, s.Dmg, s.N}
	}
	for i, s := range p.Armor {
		armor[i] = slot{s.ID, s.Dmg, s.N}
	}
	empty := true
	for _, s := range inv {
		if !s.empty() {
			empty = false
		}
	}
	if empty {
		for i := range p.Hotbar {
			if p.Counts[i] > 0 {
				inv[i] = blockSlot(p.Hotbar[i], p.Counts[i])
			}
		}
	}
	return inv, armor
}

func (o overlay) savePlayer(p playerState) error {
	return writeAtomic(filepath.Join(o.dir, "player.json"), func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(p)
	})
}

func (o overlay) loadPlayer() (playerState, bool) {
	b, err := os.ReadFile(filepath.Join(o.dir, "player.json"))
	if err != nil {
		return playerState{}, false
	}
	var p playerState
	if json.Unmarshal(b, &p) != nil || p.Selected < 0 || p.Selected > 8 {
		return playerState{}, false
	}
	return p, true
}

// saveWorld writes the edited columns and the player's state to the overlay.
func (g *game) saveWorld() {
	if g.save == nil || g.world == nil || g.opt.Bench > 0 || g.opt.Screenshot != "" { // benchmarks and screenshots must leave no trace
		return
	}
	o := overlayFor(g.save.Dir)
	g.saveDim() // the columns and block entities of the dimension being played go to its own folder
	st := playerState{Pos: g.feet, Yaw: g.yaw, Pitch: g.pitch,
		Selected: g.sel, Health: g.health, Food: g.food, Flying: g.flying, Time: g.worldTime, Dim: g.dimn.id}
	for i, s := range g.inv {
		st.Inv[i] = toJSON(s)
	}
	for i, s := range g.armor {
		st.Armor[i] = toJSON(s)
	}
	err := o.savePlayer(st)
	if err != nil {
		logf("saving the player: %v", err)
	}
}
