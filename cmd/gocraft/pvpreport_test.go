package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gocraft/proto"
)

// tools/pvpreport.py reads the lines traceLine and noteLine write. A made-up duel in that format: the player (entity
// 5) swings and attacks the rival (entity 9) three times, two of which the server answers with the rival hurt; takes
// two blows; blocks with a tap that is never released and attacks while the sword is up; the server moves it back once
// after a blow. The report must count all of that.
func TestPvPReportReadsTheLog(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	var b strings.Builder
	pkt := func(ms int64, dir string, id int32, fill func(w *proto.Writer)) {
		w := proto.NewPacket(id)
		fill(w)
		b.WriteString(traceLine(ms, dir, id, w.B[1:]))
	}
	action := func(ms int64, a int32) {
		pkt(ms, ">", 0x0B, func(w *proto.Writer) { w.VarInt(5); w.VarInt(a); w.VarInt(0) })
	}
	pos := func(ms int64, z float64) {
		pkt(ms, ">", 0x04, func(w *proto.Writer) { w.Double(0.5); w.Double(4); w.Double(z); w.Bool(true) })
	}
	swingAttack := func(ms int64) {
		pkt(ms, ">", 0x0A, func(w *proto.Writer) {})
		pkt(ms, ">", 0x02, func(w *proto.Writer) { w.VarInt(9); w.VarInt(1) })
	}
	hurt := func(ms int64, eid int32) {
		pkt(ms, "<", 0x1A, func(w *proto.Writer) { w.Int(eid); w.Byte(2) })
	}
	useAir := func(ms int64) {
		pkt(ms, ">", 0x08, func(w *proto.Writer) {
			w.Long(proto.PackPos(-1, -1, -1))
			w.Byte(255)
			w.UShort(276)
			w.Byte(1)
			w.Byte(0)
			w.Byte(0)
			w.Byte(0)
		})
	}
	action(4000, 3) // sprinting: tells the report which entity is the player
	pos(4050, 1)
	swingAttack(4100)
	hurt(4150, 9)
	pkt(4300, "<", 0x08, func(w *proto.Writer) {
		w.Double(0.5)
		w.Double(4)
		w.Double(0.8)
		w.Float(0)
		w.Float(0)
		w.Byte(0x18)
	})
	hurt(4600, 5)
	swingAttack(5000)
	hurt(5040, 9)
	useAir(5200) // a tap, never released
	swingAttack(5300)
	useAir(5600)
	pkt(5700, ">", 0x07, func(w *proto.Writer) { w.Byte(5); w.Long(0); w.Byte(0) })
	hurt(6000, 5)
	b.WriteString(noteLine(6100, "click held back: miss penalty, 9 ticks left"))
	pos(9000, 1)
	dir := t.TempDir()
	log := filepath.Join(dir, "duel.net")
	if err := os.WriteFile(log, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	script, _ := filepath.Abs(filepath.Join("..", "..", "tools", "pvpreport.py"))
	out, err := exec.Command(py, "-I", script, log).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{
		"3 attacks sent",
		"2 dealt, 2 received",
		"longest combo: 1 blow in a row",
		"items used in the air: 276 x2; 1 releases",
		"no release between: 1",
		"attacks sent while an item was in use: 1",
		"attacks followed by a correction within 400 ms: 1 of 3",
		"clicks held back by the miss penalty (a click on nothing before, button still down): 1",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the report lacks %q:\n%s", want, out)
		}
	}
}
