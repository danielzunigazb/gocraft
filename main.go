package main

import (
	"flag"
	"log"
	"net"
	"strconv"

	"gocraft/server"
	"gocraft/worldgen"
)

func main() {
	addr := flag.String("addr", ":25565", "listen address")
	motd := flag.String("motd", "gocraft 1.8.9", "server list message")
	view := flag.Int("view", 6, "view distance in chunks")
	max := flag.Int("max", 20, "max players")
	seed := flag.String("seed", "", "generate the world from this seed (a number, or any text) instead of the flat world")
	survival := flag.Bool("survival", false, "survival rules: health, player versus player damage, falling (default: creative)")
	flag.Parse()

	l, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("gocraft: Minecraft %s (protocol %d) on %s", server.Version, server.Protocol, l.Addr())
	cfg := server.Config{MOTD: *motd, MaxPlayers: *max, ViewDistance: *view, Survival: *survival}
	if *seed != "" {
		n, err := strconv.ParseInt(*seed, 10, 64)
		if err != nil {
			n = int64(worldgen.JavaHash(*seed))
		}
		gen := worldgen.NewGenerator(n)
		cfg.Seed, cfg.Generator, cfg.Spawn = n, gen.Func(), gen.Spawn
		log.Printf("generating the world from seed %d", n)
	}
	s := server.New(cfg)
	log.Fatal(s.Serve(l))
}
