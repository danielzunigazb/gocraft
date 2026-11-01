package client

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"gocraft/auth"
	"gocraft/proto"
)

// An online-mode server, with real RSA and AES/CFB8: it checks that the client told the session server
// it was joining (with the digest the server computes itself), that its name is the account's, and that
// the encrypted conversation that follows works.
func TestOnlineModeHandshake(t *testing.T) {
	var mu sync.Mutex
	var joined map[string]string
	session := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]string
		json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		joined = b
		mu.Unlock()
		w.WriteHeader(204)
	}))
	defer session.Close()

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	result := make(chan string, 8)
	go func() {
		nc, err := l.Accept()
		if err != nil {
			return
		}
		defer nc.Close()
		br := bufio.NewReader(nc)
		proto.ReadPacket(br) // handshake
		_, r, _ := proto.ReadPacket(br)
		name := r.String(16)
		token := []byte{1, 2, 3, 4}
		req := proto.NewPacket(0x01)
		req.String("")
		req.VarInt(int32(len(pub)))
		req.Bytes(pub)
		req.VarInt(int32(len(token)))
		req.Bytes(token)
		nc.Write(req.Frame())
		_, r, err = proto.ReadPacket(br)
		if err != nil {
			result <- "no encryption response: " + err.Error()
			return
		}
		encSecret := append([]byte(nil), r.Bytes(int(r.VarInt()))...)
		encToken := append([]byte(nil), r.Bytes(int(r.VarInt()))...)
		secret, err1 := rsa.DecryptPKCS1v15(rand.Reader, key, encSecret)
		got, err2 := rsa.DecryptPKCS1v15(rand.Reader, key, encToken)
		if err1 != nil || err2 != nil || string(got) != string(token) {
			result <- "the verify token did not come back"
			return
		}
		mu.Lock()
		j := joined
		mu.Unlock()
		if j["serverId"] != auth.ServerHash("", secret, pub) || j["selectedProfile"] != "069a79f444e94726a5befca90e38aaf5" || j["accessToken"] != "mc-access" {
			result <- "the session server was not told the right join: " + j["serverId"]
			return
		}
		if name != "Notch" {
			result <- "logged in as " + name + ", not the account's name"
			return
		}
		// From here on everything is encrypted.
		rd, wr, _ := proto.Encrypted(nc, secret)
		br = bufio.NewReader(rd)
		ok := proto.NewPacket(0x02)
		ok.String("069a79f4-44e9-4726-a5be-fca90e38aaf5")
		ok.String("Notch")
		wr.Write(ok.Frame())
		join := proto.NewPacket(0x01)
		join.Int(1)
		join.Byte(0)
		join.Byte(0)
		join.Byte(0)
		join.Byte(20)
		join.String("default")
		join.Bool(false)
		wr.Write(join.Frame())
		for { // the settings the client sends right after joining, decrypted
			id, _, err := proto.ReadPacket(br)
			if err != nil {
				result <- "reading the client's encrypted packets: " + err.Error()
				return
			}
			if id == 0x15 {
				result <- "ok"
				return
			}
		}
	}()

	g := testGame(t)
	g.acct.client = auth.New("client-123")
	g.acct.client.EP.SessionJoin = session.URL
	g.acct.stored = &auth.Stored{RefreshToken: "r", UUID: "069a79f444e94726a5befca90e38aaf5", Name: "Notch"}
	g.acct.session = &auth.Session{AccessToken: "mc-access", Expires: time.Now().Add(time.Hour), UUID: "069a79f444e94726a5befca90e38aaf5", Name: "Notch"}
	g.acct.use = true
	if err := g.connect(l.Addr().String(), "Player"); err != nil {
		t.Fatal(err)
	}
	defer g.leaveServer()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		g.pumpNet() // the game thread answers the Join Game with the settings
		select {
		case got := <-result:
			if got != "ok" {
				t.Fatal(got)
			}
			return
		default:
		}
	}
	t.Fatal("the server never saw the client's encrypted settings")
}

// Without an account, an online-mode server is refused with a message that says what to do.
func TestOnlineModeWithoutAnAccountSaysSo(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 1024)
	pub, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	go func() {
		nc, err := l.Accept()
		if err != nil {
			return
		}
		defer nc.Close()
		br := bufio.NewReader(nc)
		proto.ReadPacket(br)
		proto.ReadPacket(br)
		req := proto.NewPacket(0x01)
		req.String("")
		req.VarInt(int32(len(pub)))
		req.Bytes(pub)
		req.VarInt(4)
		req.Bytes([]byte{1, 2, 3, 4})
		nc.Write(req.Frame())
		time.Sleep(time.Second)
	}()
	g := testGame(t)
	err := g.connect(l.Addr().String(), "Eve")
	if err == nil || !containsAll(err.Error(), "online mode", "Microsoft") {
		t.Fatalf("got %v", err)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		found := false
		for i := 0; i+len(p) <= len(s); i++ {
			if s[i:i+len(p)] == p {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
