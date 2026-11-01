// Package server is a Minecraft 1.8.9 (protocol 47) server in offline mode: status, login, a flat
// creative world, chunks around each player, chat, and breaking and placing blocks.
package server

import (
	"bufio"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"gocraft/proto"
	"gocraft/world"
)

const (
	Protocol = 47
	Version  = "1.8.9"
)

type Config struct {
	MOTD         string
	MaxPlayers   int
	ViewDistance int // chunks around the player
	// Survival turns combat on: health, damage between players, falling and the void. Without it
	// the game is creative and nobody is hurt.
	Survival bool
	// Generator makes the columns of the world (nil: the flat world). Spawn says where the first
	// player appears in it (nil: the origin). Seed is only what the generator was made from.
	Seed      int64
	Generator func(world.ChunkPos) *world.Chunk
	Spawn     func() (x, z int32)
	// SaveDir is a world folder the server keeps what players change in (the edited columns, as a 1.8.9 world,
	// and each player's position, health and inventory). Empty: nothing is kept.
	SaveDir string
}

type Server struct {
	cfg   Config
	world *world.World

	mu      sync.Mutex
	players map[int32]*Player
	nextEID int32

	manual bool // tests drive tick() themselves

	store  *store                  // the saved world, with Config.SaveDir
	edited map[world.ChunkPos]bool // columns changed since the last save
	ticks  int

	spawnOnce sync.Once
	spawnAt   [3]float64
}

func New(cfg Config) *Server {
	if cfg.MOTD == "" {
		cfg.MOTD = "gocraft 1.8.9"
	}
	if cfg.MaxPlayers <= 0 {
		cfg.MaxPlayers = 20
	}
	if cfg.ViewDistance <= 0 {
		cfg.ViewDistance = 6
	}
	w := world.New()
	if cfg.Generator != nil {
		w = world.NewFrom(cfg.Generator)
	}
	var st *store
	if cfg.SaveDir != "" { // what was saved comes first; the rest is the generator's
		st = &store{dir: cfg.SaveDir}
		gen := cfg.Generator
		w = world.NewFrom(func(cp world.ChunkPos) *world.Chunk {
			if c := st.load(cp); c != nil {
				return c
			}
			if gen != nil {
				return gen(cp)
			}
			return world.FlatChunk()
		})
	}
	return &Server{cfg: cfg, world: w, store: st, players: map[int32]*Player{}, nextEID: 1}
}

func (s *Server) Serve(l net.Listener) error {
	if !s.manual {
		stop := make(chan struct{})
		defer close(stop)
		go s.tickLoop(stop)
	}
	for {
		nc, err := l.Accept()
		if err != nil {
			return err
		}
		go s.handle(nc)
	}
}

var validName = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)

func (s *Server) handle(nc net.Conn) {
	defer nc.Close()
	br := bufio.NewReader(nc)
	nc.SetReadDeadline(time.Now().Add(15 * time.Second))
	id, r, err := proto.ReadPacket(br)
	if err != nil || id != 0 {
		return
	}
	version := r.VarInt()
	r.String(255)
	r.UShort()
	next := r.VarInt()
	if r.Err() != nil {
		return
	}
	switch next {
	case 1:
		s.status(nc, br)
	case 2:
		s.login(nc, br, version)
	}
}

func (s *Server) status(nc net.Conn, br *bufio.Reader) {
	for {
		id, r, err := proto.ReadPacket(br)
		if err != nil {
			return
		}
		switch id {
		case 0:
			s.mu.Lock()
			online := len(s.players)
			s.mu.Unlock()
			body, _ := json.Marshal(map[string]any{
				"version":     map[string]any{"name": Version, "protocol": Protocol},
				"players":     map[string]any{"max": s.cfg.MaxPlayers, "online": online, "sample": []any{}},
				"description": map[string]any{"text": s.cfg.MOTD},
			})
			p := proto.NewPacket(0)
			p.String(string(body))
			nc.Write(p.Frame())
		case 1:
			t := r.Long()
			p := proto.NewPacket(1)
			p.Long(t)
			nc.Write(p.Frame())
			return
		default:
			return
		}
	}
}

// offlineUUID is vanilla's offline-mode id: the MD5 of "OfflinePlayer:<name>" as a version 3 UUID.
func offlineUUID(name string) string {
	h := md5.Sum([]byte("OfflinePlayer:" + name))
	h[6] = h[6]&0x0F | 0x30
	h[8] = h[8]&0x3F | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func disconnectLogin(nc net.Conn, reason string) {
	p := proto.NewPacket(0)
	p.String(textJSON(reason))
	nc.Write(p.Frame())
}

func (s *Server) login(nc net.Conn, br *bufio.Reader, version int32) {
	id, r, err := proto.ReadPacket(br)
	if err != nil || id != 0 {
		return
	}
	name := r.String(16)
	if r.Err() != nil || !validName.MatchString(name) {
		disconnectLogin(nc, "Invalid name")
		return
	}
	if version != Protocol {
		disconnectLogin(nc, "This server runs Minecraft "+Version)
		return
	}
	p, err := s.join(nc, name)
	if err != nil {
		disconnectLogin(nc, err.Error())
		return
	}
	defer s.leave(p)
	p.play(s, br)
}

func (s *Server) join(nc net.Conn, name string) (*Player, error) {
	s.mu.Lock()
	if len(s.players) >= s.cfg.MaxPlayers {
		s.mu.Unlock()
		return nil, fmt.Errorf("Server full")
	}
	for _, o := range s.players {
		if o.name == name {
			s.mu.Unlock()
			return nil, fmt.Errorf("You are already connected")
		}
	}
	p := newPlayer(nc, s.nextEID, name)
	s.nextEID++
	s.players[p.eid] = p
	s.mu.Unlock()

	ok := proto.NewPacket(2)
	ok.String(p.uuid)
	ok.String(name)
	nc.Write(ok.Frame())
	log.Printf("%s joined (%s)", name, nc.RemoteAddr())
	return p, nil
}

func (s *Server) leave(p *Player) {
	if p.inWorld() && s.store != nil {
		if err := s.savePlayer(p); err != nil {
			log.Printf("save %s: %v", p.name, err)
		}
	}
	s.mu.Lock()
	delete(s.players, p.eid)
	s.mu.Unlock()
	log.Printf("%s left", p.name)
	s.Broadcast(p.destroyPacket())
	s.Broadcast(p.listPacket(4))
	s.Broadcast(chatPacket(p.name + " left the game"))
}

// Broadcast sends a framed packet to every player in the world.
func (s *Server) Broadcast(frame []byte) {
	s.mu.Lock()
	ps := make([]*Player, 0, len(s.players))
	for _, p := range s.players {
		if p.inWorld() {
			ps = append(ps, p)
		}
	}
	s.mu.Unlock()
	for _, p := range ps {
		p.send(frame)
	}
}

// textJSON is a plain chat component. Go's encoder escapes < > & by default, which the client
// would decode anyway, but readable JSON is easier to debug.
func textJSON(text string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(map[string]string{"text": text})
	return strings.TrimSpace(b.String())
}

func chatPacket(text string) []byte {
	p := proto.NewPacket(0x02)
	p.String(textJSON(text))
	p.Byte(0) // chat box
	return p.Frame()
}
