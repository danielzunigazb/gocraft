package anvil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gocraft/world"
)

// Edits is what an export lays over a copy of a world.
type Edits struct {
	Chunks map[world.ChunkPos]*world.Chunk // the columns that were changed
	Level  func(data Compound)             // changes the "Data" compound of level.dat; may be nil
}

// Export copies the world folder src to the NEW folder dst and writes the edited columns into the copy.
// It never touches src: dst must not exist yet (or be an empty folder) and must lie outside src, and src
// outside dst. It returns how many columns it wrote.
func Export(src, dst string, e Edits) (int, error) {
	src, err := filepath.Abs(src)
	if err != nil {
		return 0, err
	}
	dst, err = filepath.Abs(dst)
	if err != nil {
		return 0, err
	}
	if within(src, dst) || within(dst, src) {
		return 0, fmt.Errorf("the export folder must be a different place from the world (%s and %s overlap)", src, dst)
	}
	if ents, err := os.ReadDir(dst); err == nil && len(ents) > 0 {
		return 0, fmt.Errorf("%s already has files in it: an export goes to a new folder", dst)
	}
	if _, err := os.Stat(filepath.Join(src, "level.dat")); err != nil {
		return 0, fmt.Errorf("%s is not a world: %w", src, err)
	}

	// The edited columns, by region file.
	byRegion := map[[2]int32][]world.ChunkPos{}
	for cp := range e.Chunks {
		k := [2]int32{cp.X >> 5, cp.Z >> 5}
		byRegion[k] = append(byRegion[k], cp)
	}
	regionName := func(k [2]int32) string { return fmt.Sprintf("r.%d.%d.mca", k[0], k[1]) }
	edited := map[string][2]int32{}
	for k := range byRegion {
		edited[regionName(k)] = k
	}

	// Copy everything except what the export writes itself.
	err = filepath.WalkDir(src, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(out, 0o755)
		case rel == "session.lock", rel == "level.dat":
			return nil
		case filepath.Dir(rel) == "region" && isEdited(edited, d.Name()):
			return nil // written below, with its edits
		case !d.Type().IsRegular():
			return nil
		}
		return copyFile(p, out)
	})
	if err != nil {
		return 0, fmt.Errorf("copying the world: %w", err)
	}

	// The regions with edits: the original file (if any) with those columns replaced.
	written := 0
	now := uint32(time.Now().Unix())
	if err := os.MkdirAll(filepath.Join(dst, "region"), 0o755); err != nil {
		return 0, err
	}
	for k, cps := range byRegion {
		old, err := os.ReadFile(filepath.Join(src, "region", regionName(k)))
		if err != nil && !os.IsNotExist(err) {
			return written, err
		}
		reg, err := ReadRegion(old)
		if err != nil {
			return written, fmt.Errorf("%s: %w", regionName(k), err)
		}
		for _, cp := range cps {
			var base Compound
			if raw := reg.raw[regionIndex(cp)]; raw != nil {
				base, _ = decodeStored(raw) // an unreadable old column is replaced, not kept
			}
			if err := reg.Set(cp, ChunkNBT(cp, e.Chunks[cp], base), now); err != nil {
				return written, err
			}
			written++
		}
		b, err := reg.Bytes()
		if err != nil {
			return written, err
		}
		if err := os.WriteFile(filepath.Join(dst, "region", regionName(k)), b, 0o644); err != nil {
			return written, err
		}
	}

	if err := rewriteLevel(filepath.Join(src, "level.dat"), filepath.Join(dst, "level.dat"), e.Level); err != nil {
		return written, fmt.Errorf("level.dat: %w", err)
	}
	return written, nil
}

func isEdited(edited map[string][2]int32, name string) bool { _, ok := edited[name]; return ok }

// within reports whether path is dir itself or lies inside it.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// decodeStored reads a column as stored in a region file: a compression byte, then the NBT.
func decodeStored(raw []byte) (Compound, error) {
	if len(raw) < 2 {
		return nil, fmt.Errorf("empty column")
	}
	var r io.Reader
	var err error
	switch raw[0] {
	case 1:
		r, err = gzip.NewReader(bytes.NewReader(raw[1:]))
	case 2:
		r, err = zlib.NewReader(bytes.NewReader(raw[1:]))
	default:
		return nil, fmt.Errorf("compression %d", raw[0])
	}
	if err != nil {
		return nil, err
	}
	return ReadNBT(r)
}

// rewriteLevel copies level.dat with the edit applied to its "Data" compound, gzip-compressed as it is.
func rewriteLevel(from, to string, edit func(Compound)) error {
	f, err := os.Open(from)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	root, err := ReadNBT(zr)
	if err != nil {
		return err
	}
	if data, ok := root["Data"].(Compound); ok && edit != nil {
		edit(data)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := WriteNBT(zw, root, ""); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(to, buf.Bytes(), 0o644)
}

// Create writes a NEW world folder at dst from nothing: the given "Data" compound as level.dat and the given
// columns as region files. It is how a generated world, which has no folder of its own, is exported. dst must
// not exist yet (or be an empty folder). It returns how many columns it wrote.
func Create(dst string, data Compound, chunks map[world.ChunkPos]*world.Chunk) (int, error) {
	dst, err := filepath.Abs(dst)
	if err != nil {
		return 0, err
	}
	if ents, err := os.ReadDir(dst); err == nil && len(ents) > 0 {
		return 0, fmt.Errorf("%s already has files in it: an export goes to a new folder", dst)
	}
	if err := os.MkdirAll(filepath.Join(dst, "region"), 0o755); err != nil {
		return 0, err
	}
	byRegion := map[[2]int32]*Region{}
	now := uint32(time.Now().Unix())
	for cp, c := range chunks {
		k := [2]int32{cp.X >> 5, cp.Z >> 5}
		r := byRegion[k]
		if r == nil {
			r = &Region{}
			byRegion[k] = r
		}
		if err := r.Set(cp, ChunkNBT(cp, c, nil), now); err != nil {
			return 0, err
		}
	}
	for k, r := range byRegion {
		b, err := r.Bytes()
		if err != nil {
			return 0, err
		}
		if err := os.WriteFile(filepath.Join(dst, "region", fmt.Sprintf("r.%d.%d.mca", k[0], k[1])), b, 0o644); err != nil {
			return 0, err
		}
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := WriteNBT(zw, Compound{"Data": data}, ""); err != nil {
		return 0, err
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(dst, "level.dat"), buf.Bytes(), 0o644); err != nil {
		return 0, err
	}
	return len(chunks), nil
}
