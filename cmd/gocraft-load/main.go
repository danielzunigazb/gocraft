// gocraft-load joins a gocraft server with several bots that walk away from the spawn in different directions, so
// that the server has to make new columns for each, and reports how fast the columns arrive.
//
//	gocraft-load -addr 127.0.0.1:25570 -bots 4 -seconds 30
package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gocraft/proto"
)

type result struct {
	chunks, bytes int
	firstChunk    time.Duration
	err           error
}

func bot(addr, name string, dir [2]float64, d time.Duration) result {
	var res result
	nc, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		res.err = err
		return res
	}
	defer nc.Close()
	br := bufio.NewReader(nc)
	send := func(p *proto.Writer) { nc.Write(p.Frame()) }
	h := proto.NewPacket(0)
	h.VarInt(47)
	h.String("load")
	h.UShort(25565)
	h.VarInt(2)
	send(h)
	l := proto.NewPacket(0)
	l.String(name)
	send(l)
	var x, y, z float64
	start := time.Now()
	var wmu sync.Mutex
	done := make(chan struct{})
	moving := false
	go func() { // walk at about 5 blocks a second, away from the spawn
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				wmu.Lock()
				if moving {
					x += dir[0] * 0.25
					z += dir[1] * 0.25
					p := proto.NewPacket(0x04)
					p.Double(x)
					p.Double(y)
					p.Double(z)
					p.Bool(true)
					send(p)
				}
				wmu.Unlock()
			}
		}
	}()
	defer close(done)
	for time.Since(start) < d {
		nc.SetReadDeadline(time.Now().Add(10 * time.Second))
		id, r, err := proto.ReadPacket(br)
		if err != nil {
			res.err = err
			return res
		}
		switch id {
		case 0x08:
			wmu.Lock()
			x, y, z = r.Double(), r.Double(), r.Double()
			moving = true
			wmu.Unlock()
		case 0x21, 0x26:
			if res.chunks == 0 {
				res.firstChunk = time.Since(start)
			}
			res.chunks++
			res.bytes += r.Len()
		case 0x00: // keep alive
			k := proto.NewPacket(0x00)
			k.VarInt(r.VarInt())
			send(k)
		}
	}
	return res
}

func main() {
	addr := flag.String("addr", "127.0.0.1:25565", "the server")
	bots := flag.Int("bots", 4, "how many bots")
	secs := flag.Int("seconds", 30, "how long they walk")
	flag.Parse()
	dirs := [][2]float64{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {0.7, 0.7}, {-0.7, 0.7}, {0.7, -0.7}, {-0.7, -0.7}}
	var wg sync.WaitGroup
	results := make([]result, *bots)
	var joined atomic.Int32
	t0 := time.Now()
	for i := 0; i < *bots; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			joined.Add(1)
			results[i] = bot(*addr, fmt.Sprintf("Bot%d", i), dirs[i%len(dirs)], time.Duration(*secs)*time.Second)
		}(i)
	}
	wg.Wait()
	el := time.Since(t0).Seconds()
	var total, bytes int
	var firsts []time.Duration
	failed := 0
	for i, r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "bot %d: %v\n", i, r.err)
			failed++
		}
		total += r.chunks
		bytes += r.bytes
		if r.chunks > 0 {
			firsts = append(firsts, r.firstChunk)
		}
	}
	sort.Slice(firsts, func(i, j int) bool { return firsts[i] < firsts[j] })
	fmt.Printf("bots %d (failed %d), %.0f s: %d columns, %.1f columns/s, %.1f MB\n", *bots, failed, el, total, float64(total)/el, float64(bytes)/1e6)
	if len(firsts) > 0 {
		fmt.Printf("first column after %v (best) .. %v (worst)\n", firsts[0].Round(time.Millisecond), firsts[len(firsts)-1].Round(time.Millisecond))
	}
	if failed > 0 {
		os.Exit(1)
	}
}
