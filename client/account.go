package client

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gocraft/auth"
)

// The Microsoft account of the player, for online-mode servers. gocraft carries no Azure application
// of its own: the client ID comes from GOCRAFT_MSA_CLIENT_ID or from msaClientId in options.txt (see
// ACCOUNT.md). The refresh token is kept in $XDG_CONFIG_HOME/gocraft/account.json (0600); the Minecraft
// access token only in memory.

type account struct {
	mu      sync.Mutex
	client  *auth.Client
	stored  *auth.Stored
	session *auth.Session
	code    *auth.DeviceCode // the code on screen while a sign-in waits for the player
	status  string
	busy    bool
	use     bool // play with the account (online mode) rather than the offline name
	cancel  context.CancelFunc
}

func configDir() string { return filepath.Dir(filepath.Dir(optionsPath())) }

func accountPath() string { return auth.Path(configDir()) }

// acctInit reads the client ID and the saved account, once.
func (g *game) acctInit() {
	a := &g.acct
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client != nil || a.stored != nil || a.status != "" {
		return
	}
	id := strings.TrimSpace(os.Getenv("GOCRAFT_MSA_CLIENT_ID"))
	if id == "" {
		id = strings.TrimSpace(g.opts.MSAClientID)
	}
	if id != "" {
		a.client = auth.New(id)
	}
	if st, err := auth.Load(accountPath()); err == nil && st != nil {
		a.stored, a.use = st, true
	}
	switch {
	case a.client == nil:
		a.status = "No Microsoft application set up: see ACCOUNT.md"
	case a.stored == nil:
		a.status = "Not signed in"
	default:
		a.status = "Signed in as " + a.stored.Name
	}
}

// acctSignIn starts the device-code sign-in; the screen shows the code while the player enters it.
func (g *game) acctSignIn() {
	g.acctInit()
	a := &g.acct
	a.mu.Lock()
	if a.busy || a.client == nil {
		a.mu.Unlock()
		return
	}
	a.busy, a.status = true, "Asking Microsoft..."
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	a.cancel = cancel
	client := a.client
	a.mu.Unlock()
	go func() {
		defer cancel()
		fail := func(err error) {
			a.mu.Lock()
			a.busy, a.code, a.status = false, nil, err.Error()
			a.mu.Unlock()
		}
		dc, err := client.StartDeviceCode(ctx)
		if err != nil {
			fail(err)
			return
		}
		a.mu.Lock()
		a.code, a.status = dc, "Waiting for you to finish in the browser..."
		a.mu.Unlock()
		ms, err := client.PollDeviceCode(ctx, dc)
		if err != nil {
			fail(err)
			return
		}
		a.mu.Lock()
		a.code, a.status = nil, "Signing in to Minecraft..."
		a.mu.Unlock()
		sess, err := client.Login(ctx, ms)
		if err != nil {
			fail(err)
			return
		}
		st := &auth.Stored{RefreshToken: ms.Refresh, UUID: sess.UUID, Name: sess.Name}
		saveErr := auth.Save(accountPath(), st)
		a.mu.Lock()
		a.busy, a.stored, a.session, a.use = false, st, sess, true
		a.status = "Signed in as " + sess.Name
		if saveErr != nil {
			a.status += " (not saved: " + saveErr.Error() + ")"
		}
		a.mu.Unlock()
	}()
}

// acctSignOut forgets the account.
func (g *game) acctSignOut() {
	a := &g.acct
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
	a.busy, a.code, a.stored, a.session, a.use = false, nil, nil, nil, false
	_ = auth.Delete(accountPath())
	a.status = "Not signed in"
	if a.client == nil {
		a.status = "No Microsoft application set up: see ACCOUNT.md"
	}
}

// onlineLogin is what a connection uses to prove who the player is: a fresh Minecraft session of the
// saved account, or nil when the player is not signed in or chose their offline name.
func (g *game) onlineLogin() *loginAuth {
	a := &g.acct
	a.mu.Lock()
	if a.client == nil || a.stored == nil || !a.use || a.busy {
		a.mu.Unlock()
		return nil
	}
	if a.session != nil && time.Until(a.session.Expires) > 5*time.Minute {
		la := &loginAuth{Client: a.client, Session: a.session}
		a.mu.Unlock()
		return la
	}
	client, refresh := a.client, a.stored.RefreshToken
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	ms, err := client.Refresh(ctx, refresh)
	var sess *auth.Session
	if err == nil {
		sess, err = client.Login(ctx, ms)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.status = err.Error()
		if err == auth.ErrSignInAgain {
			a.stored, a.session, a.use = nil, nil, false
			_ = auth.Delete(accountPath())
		}
		return nil
	}
	a.session = sess
	if ms.Refresh != "" && ms.Refresh != refresh { // the refresh token rotates
		a.stored.RefreshToken = ms.Refresh
		_ = auth.Save(accountPath(), a.stored)
	}
	return &loginAuth{Client: client, Session: sess}
}

// acctView is what the Multiplayer screen shows of the account.
func (g *game) acctView() (status string, code *auth.DeviceCode, signedIn, busy, haveClient, use bool) {
	g.acctInit()
	a := &g.acct
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status, a.code, a.stored != nil, a.busy, a.client != nil, a.use
}
