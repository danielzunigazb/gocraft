package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"gocraft/server"
	"gocraft/worldgen"
)

// version is set by the release build (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	addr := flag.String("addr", ":25565", "listen address")
	motd := flag.String("motd", "gocraft 1.8.9", "server list message")
	view := flag.Int("view", 6, "view distance in chunks")
	max := flag.Int("max", 20, "max players")
	seed := flag.String("seed", "", "generate the world from this seed (a number, or any text) instead of the flat world")
	survival := flag.Bool("survival", false, "survival rules: health, player versus player damage, falling (default: creative)")
	saveDir := flag.String("save", "", "keep the world and the players in this folder (a 1.8.9 world folder): the edited columns and each player's position and inventory")
	flag.Parse()
	if *showVersion {
		fmt.Println("gocraft-server", version)
		return
	}

	l, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("gocraft: Minecraft %s (protocol %d) on %s", server.Version, server.Protocol, l.Addr())
	cfg := server.Config{MOTD: *motd, MaxPlayers: *max, ViewDistance: *view, Survival: *survival, SaveDir: *saveDir}
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
	if *saveDir != "" { // the world is written on a stop signal too, not only every minute
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-stop
			if err := s.Save(); err != nil {
				log.Printf("save: %v", err)
			}
			log.Printf("saved to %s", *saveDir)
			os.Exit(0)
		}()
	}
	log.Fatal(s.Serve(l))
}
