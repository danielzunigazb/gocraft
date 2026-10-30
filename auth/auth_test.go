package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fake is a stand-in for Microsoft, Xbox Live, the Minecraft services and the session server.
type fake struct {
	t        *testing.T
	polls    atomic.Int32
	xerr     int64
	status   map[string]int // endpoint name -> forced status
	items    string
	joinBody map[string]string
}

func (f *fake) handler() http.Handler {
	mux := http.NewServeMux()
	jsonOut := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/devicecode", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("client_id") != "client-123" || r.Form.Get("scope") != "XboxLive.signin offline_access" {
			f.t.Errorf("device code request %v", r.Form)
		}
		jsonOut(w, 200, map[string]any{"device_code": "dev", "user_code": "ABCD1234", "verification_uri": "https://www.microsoft.com/link", "expires_in": 900, "interval": 1})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "urn:ietf:params:oauth:grant-type:device_code":
			if r.Form.Get("device_code") != "dev" {
				f.t.Errorf("poll with %v", r.Form)
			}
			if f.polls.Add(1) == 1 {
				jsonOut(w, 400, map[string]any{"error": "authorization_pending"})
				return
			}
			jsonOut(w, 200, map[string]any{"access_token": "ms-access", "refresh_token": "ms-refresh", "expires_in": 3600})
		case "refresh_token":
			if r.Form.Get("refresh_token") == "stale" {
				jsonOut(w, 400, map[string]any{"error": "invalid_grant"})
				return
			}
			jsonOut(w, 200, map[string]any{"access_token": "ms-access2", "refresh_token": "ms-refresh2", "expires_in": 3600})
		default:
			f.t.Errorf("grant %q", r.Form.Get("grant_type"))
		}
	})
	mux.HandleFunc("/xbl", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Properties   map[string]string
			RelyingParty string
			TokenType    string
		}
		json.NewDecoder(r.Body).Decode(&b)
		if !strings.HasPrefix(b.Properties["RpsTicket"], "d=ms-access") || b.Properties["AuthMethod"] != "RPS" ||
			b.Properties["SiteName"] != "user.auth.xboxlive.com" || b.RelyingParty != "http://auth.xboxlive.com" {
			f.t.Errorf("xbl request %+v", b)
		}
		jsonOut(w, 200, map[string]any{"Token": "xbl-token", "DisplayClaims": map[string]any{"xui": []map[string]string{{"uhs": "uhs-1"}}}})
	})
	mux.HandleFunc("/xsts", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Properties   struct{ UserTokens []string }
			RelyingParty string
		}
		json.NewDecoder(r.Body).Decode(&b)
		if len(b.Properties.UserTokens) != 1 || b.Properties.UserTokens[0] != "xbl-token" || b.RelyingParty != "rp://api.minecraftservices.com/" {
			f.t.Errorf("xsts request %+v", b)
		}
		if f.xerr != 0 {
			jsonOut(w, 401, map[string]any{"XErr": f.xerr})
			return
		}
		jsonOut(w, 200, map[string]any{"Token": "xsts-token", "DisplayClaims": map[string]any{"xui": []map[string]string{{"uhs": "uhs-1"}}}})
	})
	mux.HandleFunc("/mclogin", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ IdentityToken string }
		json.NewDecoder(r.Body).Decode(&b)
		if b.IdentityToken != "XBL3.0 x=uhs-1;xsts-token" {
			f.t.Errorf("identity token %q", b.IdentityToken)
		}
		if c := f.status["mclogin"]; c != 0 {
			jsonOut(w, c, map[string]any{"error": "Invalid app registration"})
			return
		}
		jsonOut(w, 200, map[string]any{"access_token": "mc-access", "expires_in": 86400})
	})
	bearer := func(r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mc-access" {
			f.t.Errorf("%s authorization %q", r.URL.Path, r.Header.Get("Authorization"))
		}
	}
	mux.HandleFunc("/entitlements", func(w http.ResponseWriter, r *http.Request) {
		bearer(r)
		var items []map[string]string
		for _, n := range strings.Fields(f.items) {
			items = append(items, map[string]string{"name": n})
		}
		jsonOut(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("/profile", func(w http.ResponseWriter, r *http.Request) {
		bearer(r)
		if f.status["profile"] == 404 {
			jsonOut(w, 404, map[string]any{"error": "NOT_FOUND"})
			return
		}
		jsonOut(w, 200, map[string]any{"id": "069a79f444e94726a5befca90e38aaf5", "name": "Notch"})
	})
	mux.HandleFunc("/join", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&f.joinBody)
		w.WriteHeader(204)
	})
	return mux
}

func newFake(t *testing.T) (*Client, *fake) {
	f := &fake{t: t, items: "game_minecraft product_minecraft", status: map[string]int{}}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	c := New("client-123")
	c.EP = Endpoints{DeviceCode: srv.URL + "/devicecode", Token: srv.URL + "/token", XBL: srv.URL + "/xbl", XSTS: srv.URL + "/xsts",
		MinecraftLogin: srv.URL + "/mclogin", Entitlements: srv.URL + "/entitlements", Profile: srv.URL + "/profile", SessionJoin: srv.URL + "/join"}
	return c, f
}

func TestFullSignIn(t *testing.T) {
	c, f := newFake(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dc, err := c.StartDeviceCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dc.UserCode != "ABCD1234" || dc.VerificationURI != "https://www.microsoft.com/link" {
		t.Fatalf("device code %+v", dc)
	}
	ms, err := c.PollDeviceCode(ctx, dc) // the first poll is "authorization_pending"
	if err != nil {
		t.Fatal(err)
	}
	if ms.Access != "ms-access" || ms.Refresh != "ms-refresh" || f.polls.Load() != 2 {
		t.Fatalf("tokens %+v after %d polls", ms, f.polls.Load())
	}
	s, err := c.Login(ctx, ms)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Notch" || s.UUID != "069a79f444e94726a5befca90e38aaf5" || s.AccessToken != "mc-access" {
		t.Fatalf("session %+v", s)
	}
	if err := c.Join(ctx, s, "abc"); err != nil {
		t.Fatal(err)
	}
	if f.joinBody["accessToken"] != "mc-access" || f.joinBody["selectedProfile"] != s.UUID || f.joinBody["serverId"] != "abc" {
		t.Fatalf("join body %v", f.joinBody)
	}
}

func TestRefresh(t *testing.T) {
	c, _ := newFake(t)
	got, err := c.Refresh(context.Background(), "ms-refresh")
	if err != nil || got.Access != "ms-access2" || got.Refresh != "ms-refresh2" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := c.Refresh(context.Background(), "stale"); err != ErrSignInAgain {
		t.Fatalf("a refused refresh token gave %v", err)
	}
}

func TestSignInProblemsAreSaidPlainly(t *testing.T) {
	ms := &Tokens{Access: "ms-access"}
	t.Run("not approved by Mojang", func(t *testing.T) {
		c, f := newFake(t)
		f.status["mclogin"] = 403
		if _, err := c.Login(context.Background(), ms); err != ErrNotApproved {
			t.Fatal(err)
		}
	})
	t.Run("does not own the game", func(t *testing.T) {
		c, f := newFake(t)
		f.items = ""
		if _, err := c.Login(context.Background(), ms); err != ErrNoOwner {
			t.Fatal(err)
		}
	})
	t.Run("no profile yet", func(t *testing.T) {
		c, f := newFake(t)
		f.status["profile"] = 404
		if _, err := c.Login(context.Background(), ms); err != ErrNoProfile {
			t.Fatal(err)
		}
	})
	t.Run("child account", func(t *testing.T) {
		c, f := newFake(t)
		f.xerr = 2148916238
		_, err := c.Login(context.Background(), ms)
		if err == nil || !strings.Contains(err.Error(), "child account") {
			t.Fatal(err)
		}
	})
	t.Run("no client id", func(t *testing.T) {
		if _, err := New("").StartDeviceCode(context.Background()); err != ErrNoClientID {
			t.Fatal(err)
		}
	})
}

// The session server's digest: the known vectors from the protocol notes.
func TestServerHash(t *testing.T) {
	for in, want := range map[string]string{
		"Notch": "4ed1f46bbe04bc756bcb17c0c7ce3e4632f06a48",
		"jeb_":  "-7c9d5b0044c130109a5d7b5fb5c317c02b4e28c1",
		"simon": "88e16a1019277b15d58faf0541e11910eb756f6",
	} {
		if got := ServerHash(in, nil, nil); got != want {
			t.Errorf("hash of %q = %s, want %s", in, got, want)
		}
	}
}

func TestStoreIsPrivateAndRoundTrips(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gocraft", "account.json")
	if got, err := Load(p); got != nil || err != nil {
		t.Fatalf("an empty store gave %v %v", got, err)
	}
	if err := Save(p, &Stored{RefreshToken: "r", UUID: "u", Name: "n"}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	got, _ := Load(p)
	if got == nil || got.RefreshToken != "r" || got.Name != "n" {
		t.Fatalf("%+v", got)
	}
	if err := Delete(p); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(p); got != nil {
		t.Fatal("still there after sign-out")
	}
	_ = url.Values{}
}
