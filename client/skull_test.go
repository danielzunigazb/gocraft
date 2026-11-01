package client

import (
	"bytes"
	"encoding/base64"
	"testing"

	"gocraft/anvil"
	"gocraft/proto"
)

const testSkinURL = "http://textures.minecraft.net/texture/abc123"

func ownerCompound(url string) anvil.Compound {
	j := `{"timestamp":1,"profileId":"x","profileName":"y","textures":{"SKIN":{"url":"` + url + `"}}}`
	return anvil.Compound{
		"Id": "00000000-0000-0000-0000-000000000001", "Name": "Alguien",
		"Properties": anvil.Compound{"textures": []any{anvil.Compound{"Value": base64.StdEncoding.EncodeToString([]byte(j))}}},
	}
}

func nbtBytes(t *testing.T, c anvil.Compound) []byte {
	var b bytes.Buffer
	if err := anvil.WriteNBT(&b, c, ""); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func blockEntityPacket(t *testing.T, x, y, z int32, action byte, c anvil.Compound) *proto.Writer {
	w := newPacket(0x35)
	w.Long(proto.PackPos(x, y, z))
	w.Byte(action)
	w.Bytes(nbtBytes(t, c))
	return w
}

// The skin is the URL inside the base64 JSON of the owner's profile, and only Mojang's own hosts are believed.
func TestSkullSkinURL(t *testing.T) {
	if got := skullSkinURL(ownerCompound(testSkinURL)); got != testSkinURL {
		t.Fatalf("skin %q", got)
	}
	if got := skullSkinURL(ownerCompound("http://evil.example.com/skin.png")); got != "" {
		t.Fatalf("a skin from another host was accepted: %q", got)
	}
	if got := skullSkinURL(anvil.Compound{"Name": "Solo nombre"}); got != "" {
		t.Fatalf("a bare name gave %q", got)
	}
}

// Update Block Entity (action 4) records a skull's type, turn and owner; other actions are ignored.
func TestSkullBlockEntityPacket(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(blockEntityPacket(t, 3, 5, 4, 4, anvil.Compound{"SkullType": int8(3), "Rot": int8(21), "Owner": ownerCompound(testSkinURL)}))
	s := g.skulls[[3]int32{3, 5, 4}]
	if s == nil || s.typ != 3 || s.rot != 5 || s.skin != testSkinURL {
		t.Fatalf("skull %+v", s)
	}
	if f := decodeFrame(t, blockEntityPacket(t, 1, 1, 1, 1, anvil.Compound{"SpawnPotentials": int8(0)}), nil); f != nil {
		f(g)
	}
	if len(g.skulls) != 1 {
		t.Fatalf("%d skulls after a spawner update", len(g.skulls))
	}
}

// The head sits where TileEntitySkullRenderer puts it: in the middle of the floor, or against the wall it is on.
func TestSkullPlacement(t *testing.T) {
	p := [3]int32{10, 5, 20}
	for _, d := range []struct {
		facing, rot int
		at          vec3
		deg         float32
	}{
		{1, 4, vec3{10.5, 5, 20.5}, 90}, {2, 0, vec3{10.5, 5.25, 20.74}, 0}, {3, 0, vec3{10.5, 5.25, 20.26}, 180},
		{4, 0, vec3{10.74, 5.25, 20.5}, 270}, {5, 0, vec3{10.26, 5.25, 20.5}, 90},
	} {
		at, deg := skullPlace(p, d.facing, d.rot)
		if !near(at[0], d.at[0], 1e-9) || !near(at[1], d.at[1], 1e-9) || !near(at[2], d.at[2], 1e-9) || deg != d.deg {
			t.Fatalf("facing %d: %v %v, want %v %v", d.facing, at, deg, d.at, d.deg)
		}
	}
}

// A skull block is a box of 8 pixels (a humanoid head's second layer a quarter pixel larger), on the texture of
// its type.
func TestSkullBlockGeometry(t *testing.T) {
	g, _, apply := mobGame(t)
	g.camPos = vec3{10, 5, 20}
	g.setBlock(10, 5, 20, idSkull<<4|1)
	apply(blockEntityPacket(t, 10, 5, 20, 4, anvil.Compound{"SkullType": int8(0), "Rot": int8(0)}))
	groups := map[string][]vertex{}
	g.emitSkulls(groups)
	vs := groups["skeleton/skeleton"]
	if len(vs) != 24 {
		t.Fatalf("a skeleton skull has %d vertices, want 24", len(vs))
	}
	lo, hi := standBounds(vs)
	if !near(float64(hi[0]-lo[0]), 0.5, 1e-4) || !near(float64(lo[1]), 5, 1e-4) || !near(float64(hi[1]), 5.5, 1e-4) {
		t.Fatalf("skull spans %v..%v", lo, hi)
	}
	// a player's has two layers, and wears Steve's skin until its own arrives
	g.setBlock(12, 5, 20, idSkull<<4|2) // on a wall facing north: against the south side of its block
	apply(blockEntityPacket(t, 12, 5, 20, 4, anvil.Compound{"SkullType": int8(3), "Rot": int8(0)}))
	groups = map[string][]vertex{}
	g.emitSkulls(groups)
	vs = groups[g.defaultSkinKey(false)]
	if len(vs) != 48 {
		t.Fatalf("a player's skull has %d vertices, want 48 (%v)", len(vs), len(groups))
	}
	lo, hi = standBounds(vs)
	if !near(float64(lo[1]), 5.25-0.5/16*0.5, 0.01) || !near(float64((lo[2]+hi[2])/2), 20.74, 1e-4) {
		t.Fatalf("wall skull spans %v..%v", lo, hi)
	}
	// a skull whose block was replaced is not drawn
	g.setBlock(10, 5, 20, 0)
	groups = map[string][]vertex{}
	g.emitSkulls(groups)
	if len(groups["skeleton/skeleton"]) != 0 || g.skulls[[3]int32{10, 5, 20}] != nil {
		t.Fatal("a skull outlived its block")
	}
}

// A skull as the helmet of an armor stand is drawn at 1.1875 of its size from the head's pivot.
func TestArmorStandWearsASkull(t *testing.T) {
	g, _, apply := mobGame(t)
	g.partial = 1
	apply(spawnMobPacket(40, 30, 3.5, 4, 3.5, 0, 0, nil))
	w := newPacket(0x04)
	w.VarInt(40)
	w.UShort(4)
	w.UShort(idSkullItem)
	w.Byte(1)
	w.UShort(4) // a creeper's
	w.Bytes(nbtBytes(t, anvil.Compound{"SkullOwner": ownerCompound(testSkinURL)}))
	apply(w)
	e := g.net.entity(40)
	if !e.stand.head.on || e.stand.head.typ != 4 || e.stand.head.skin != testSkinURL {
		t.Fatalf("head %+v", e.stand.head)
	}
	e.prev = e.pos
	groups := map[string][]vertex{}
	g.emitStand(groups, e, false)
	vs := groups["creeper/creeper"]
	if len(vs) != 24 {
		t.Fatalf("the skull has %d vertices, want 24", len(vs))
	}
	lo, hi := standBounds(vs)
	pivot := 4 + 1.5078125 - 1.0/16
	if !near(float64(lo[1]), pivot, 1e-3) || !near(float64(hi[1]), pivot+8*1.1875/16, 1e-3) || !near(float64(hi[0]-lo[0]), 8*1.1875/16, 1e-3) {
		t.Fatalf("skull spans %v..%v, want y %v..%v", lo, hi, pivot, pivot+8*1.1875/16)
	}
	apply(equipPacket(40, 4, 0xFFFF)) // taking it off
	if e.stand.head.on {
		t.Fatal("the skull stayed on")
	}
}

// On a small stand the head is a child's: three quarters of its size, and lowered with it.
func TestSmallArmorStandWearsASkull(t *testing.T) {
	g, _, apply := mobGame(t)
	g.partial = 1
	apply(spawnMobPacket(41, 30, 3.5, 4, 3.5, 0, 0, metaByte(10, 1)))
	w := newPacket(0x04)
	w.VarInt(41)
	w.UShort(4)
	w.UShort(idSkullItem)
	w.Byte(1)
	w.UShort(0) // a skeleton's
	w.Byte(0)
	apply(w)
	e := g.net.entity(41)
	e.prev = e.pos
	groups := map[string][]vertex{}
	g.emitStand(groups, e, true)
	lo, hi := standBounds(groups["skeleton/skeleton"])
	pivot := 4 + childHeadShift + childHeadScale*(1.5078125-1.0/16)
	if !near(float64(lo[1]), pivot, 1e-3) || !near(float64(hi[1]), pivot+childHeadScale*1.1875*8/16, 1e-3) {
		t.Fatalf("small stand's skull spans %v..%v, want y %v..", lo[1], hi[1], pivot)
	}
}

// A player's head wears Steve's skin until the owner's has been fetched, then that one.
func TestSkullTextureFollowsTheSkin(t *testing.T) {
	g, _, _ := mobGame(t)
	if got := g.skullTexture(3, testSkinURL); got != g.defaultSkinKey(false) {
		t.Fatalf("before the fetch: %q", got)
	}
	g.skins = map[string]*skinSlot{testSkinURL: {state: 1}}
	if got := g.skullTexture(3, testSkinURL); got != skinKey(testSkinURL) {
		t.Fatalf("after the fetch: %q", got)
	}
	if got := g.skullTexture(1, testSkinURL); got != "skeleton/wither_skeleton" {
		t.Fatalf("a wither skeleton's: %q", got)
	}
}
