// Package auth signs a player in with their own Microsoft account, as the Minecraft launcher does,
// and proves it to an online-mode server. The flow is the same one Horizon's launcher uses:
//
//	device code -> Microsoft token -> Xbox Live -> XSTS -> Minecraft services -> profile
//
// gocraft has no Azure application of its own baked in. The player registers one (a public client,
// device-code flow, "Personal Microsoft accounts only"), has Mojang approve it, and gives its client
// ID in GOCRAFT_MSA_CLIENT_ID or in options.txt: see ACCOUNT.md. Never use another application's ID.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Endpoints are the services a sign-in talks to; tests point them at a fake.
type Endpoints struct {
	DeviceCode, Token, XBL, XSTS, MinecraftLogin, Entitlements, Profile, SessionJoin string
}

// DefaultEndpoints are the real ones.
func DefaultEndpoints() Endpoints {
	return Endpoints{
		DeviceCode:     "https://login.microsoftonline.com/consumers/oauth2/v2.0/devicecode",
		Token:          "https://login.microsoftonline.com/consumers/oauth2/v2.0/token",
		XBL:            "https://user.auth.xboxlive.com/user/authenticate",
		XSTS:           "https://xsts.auth.xboxlive.com/xsts/authorize",
		MinecraftLogin: "https://api.minecraftservices.com/authentication/login_with_xbox",
		Entitlements:   "https://api.minecraftservices.com/entitlements/mcstore",
		Profile:        "https://api.minecraftservices.com/minecraft/profile",
		SessionJoin:    "https://sessionserver.mojang.com/session/minecraft/join",
	}
}

const scope = "XboxLive.signin offline_access"

// Client runs the sign-in for one registered application.
type Client struct {
	ClientID string
	EP       Endpoints
	HTTP     *http.Client
}

// New returns a client for the application with the given client ID.
func New(clientID string) *Client {
	return &Client{ClientID: clientID, EP: DefaultEndpoints(), HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// Errors the player can act on. Their text is shown as it is.
var (
	ErrNoClientID  = errors.New("gocraft has no Microsoft application to sign in with: register one and set GOCRAFT_MSA_CLIENT_ID (see ACCOUNT.md)")
	ErrNotApproved = errors.New("Mojang has not approved this application yet (https://aka.ms/mce-reviewappid)")
	ErrNoOwner     = errors.New("this account doesn't own Minecraft: Java Edition")
	ErrNoProfile   = errors.New("create your Minecraft profile on minecraft.net first")
	ErrSignInAgain = errors.New("the saved sign-in expired: sign in again")
	ErrDeclined    = errors.New("the sign-in was declined")
	ErrExpired     = errors.New("the code expired: start the sign-in again")
)

// DeviceCode is what the player is shown: a short code to enter at a web address.
type DeviceCode struct {
	UserCode        string
	VerificationURI string
	ExpiresIn       time.Duration
	Interval        time.Duration
	deviceCode      string
}

// Tokens are the Microsoft tokens of a sign-in. The refresh token is the long-lived secret.
type Tokens struct {
	Access, Refresh string
	Expires         time.Time
}

// Session is what an online-mode server needs: a Minecraft access token and who it belongs to.
type Session struct {
	AccessToken string
	Expires     time.Time
	UUID        string // without dashes
	Name        string
}

func (c *Client) post(ctx context.Context, endpoint string, form url.Values, js any, out any, headers map[string]string) (int, error) {
	var body io.Reader
	ctype := ""
	if form != nil {
		body, ctype = strings.NewReader(form.Encode()), "application/x-www-form-urlencoded"
	} else if js != nil {
		b, _ := json.Marshal(js)
		body, ctype = strings.NewReader(string(b)), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return 0, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.do(req, out)
}

func (c *Client) get(ctx context.Context, endpoint, bearer string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	return c.do(req, out)
}

// do sends a request and decodes a JSON answer into out. Errors name the endpoint's host, never a token.
func (c *Client) do(req *http.Request, out any) (int, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // the transport's own reason, without the full address
		}
		return 0, fmt.Errorf("%s: %w", req.URL.Host, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if out != nil && len(b) > 0 {
		_ = json.Unmarshal(b, out)
	}
	return resp.StatusCode, nil
}

type oauthReply struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	DeviceCode       string `json:"device_code"`
	UserCode         string `json:"user_code"`
	VerificationURI  string `json:"verification_uri"`
	Interval         int    `json:"interval"`
}

// StartDeviceCode asks Microsoft for a code the player enters at VerificationURI.
func (c *Client) StartDeviceCode(ctx context.Context) (*DeviceCode, error) {
	if c.ClientID == "" {
		return nil, ErrNoClientID
	}
	var r oauthReply
	st, err := c.post(ctx, c.EP.DeviceCode, url.Values{"client_id": {c.ClientID}, "scope": {scope}}, nil, &r, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 || r.DeviceCode == "" {
		return nil, fmt.Errorf("Microsoft refused the sign-in request (%s)", firstNonEmpty(r.ErrorDescription, r.Error, fmt.Sprint("HTTP ", st)))
	}
	iv := time.Duration(r.Interval) * time.Second
	if iv <= 0 {
		iv = 5 * time.Second
	}
	return &DeviceCode{UserCode: r.UserCode, VerificationURI: r.VerificationURI,
		ExpiresIn: time.Duration(r.ExpiresIn) * time.Second, Interval: iv, deviceCode: r.DeviceCode}, nil
}

// PollDeviceCode waits for the player to finish at the web address. It returns when they have, when they
// decline, when the code expires or when ctx ends.
func (c *Client) PollDeviceCode(ctx context.Context, dc *DeviceCode) (*Tokens, error) {
	interval := dc.Interval
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		var r oauthReply
		_, err := c.post(ctx, c.EP.Token, url.Values{
			"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":  {c.ClientID}, "device_code": {dc.deviceCode},
		}, nil, &r, nil)
		if err != nil {
			return nil, err
		}
		switch r.Error {
		case "":
			return &Tokens{Access: r.AccessToken, Refresh: r.RefreshToken, Expires: time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)}, nil
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "authorization_declined":
			return nil, ErrDeclined
		case "expired_token", "bad_verification_code":
			return nil, ErrExpired
		default:
			return nil, fmt.Errorf("Microsoft: %s", firstNonEmpty(r.ErrorDescription, r.Error))
		}
	}
}

// Refresh renews the Microsoft tokens from a refresh token.
func (c *Client) Refresh(ctx context.Context, refresh string) (*Tokens, error) {
	if c.ClientID == "" {
		return nil, ErrNoClientID
	}
	var r oauthReply
	_, err := c.post(ctx, c.EP.Token, url.Values{
		"grant_type": {"refresh_token"}, "client_id": {c.ClientID}, "refresh_token": {refresh}, "scope": {scope},
	}, nil, &r, nil)
	if err != nil {
		return nil, err
	}
	if r.Error == "invalid_grant" {
		return nil, ErrSignInAgain
	}
	if r.Error != "" || r.AccessToken == "" {
		return nil, fmt.Errorf("Microsoft: %s", firstNonEmpty(r.ErrorDescription, r.Error))
	}
	return &Tokens{Access: r.AccessToken, Refresh: firstNonEmpty(r.RefreshToken, refresh),
		Expires: time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)}, nil
}

type xboxReply struct {
	Token         string `json:"Token"`
	DisplayClaims struct {
		Xui []struct {
			Uhs string `json:"uhs"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
	XErr int64 `json:"XErr"`
}

// xstsMessages are the XErr codes a player can fix, as Horizon words them.
var xstsMessages = map[int64]string{
	2148916233: "this Microsoft account has no Xbox profile yet: create one at xbox.com",
	2148916235: "Xbox Live is not available in your country",
	2148916236: "this account needs adult verification (South Korea)",
	2148916237: "this account needs adult verification (South Korea)",
	2148916238: "this is a child account: it needs to be added to a family",
}

// Login turns Microsoft tokens into a Minecraft session: Xbox Live, XSTS, Minecraft services, then the
// account must own the game and have a profile.
func (c *Client) Login(ctx context.Context, ms *Tokens) (*Session, error) {
	var xbl xboxReply
	st, err := c.post(ctx, c.EP.XBL, nil, map[string]any{
		"Properties":   map[string]any{"AuthMethod": "RPS", "SiteName": "user.auth.xboxlive.com", "RpsTicket": "d=" + ms.Access},
		"RelyingParty": "http://auth.xboxlive.com", "TokenType": "JWT",
	}, &xbl, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 || xbl.Token == "" || len(xbl.DisplayClaims.Xui) == 0 {
		return nil, fmt.Errorf("Xbox Live refused the sign-in (HTTP %d)", st)
	}
	uhs := xbl.DisplayClaims.Xui[0].Uhs

	var xsts xboxReply
	st, err = c.post(ctx, c.EP.XSTS, nil, map[string]any{
		"Properties":   map[string]any{"SandboxId": "RETAIL", "UserTokens": []string{xbl.Token}},
		"RelyingParty": "rp://api.minecraftservices.com/", "TokenType": "JWT",
	}, &xsts, nil)
	if err != nil {
		return nil, err
	}
	if msg, ok := xstsMessages[xsts.XErr]; ok {
		return nil, errors.New(msg)
	}
	if st != 200 || xsts.Token == "" {
		return nil, fmt.Errorf("Xbox Live (XSTS) refused the sign-in (HTTP %d)", st)
	}

	var mc struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	st, err = c.post(ctx, c.EP.MinecraftLogin, nil, map[string]any{"identityToken": "XBL3.0 x=" + uhs + ";" + xsts.Token}, &mc, nil)
	if err != nil {
		return nil, err
	}
	if st == 403 {
		return nil, ErrNotApproved
	}
	if st != 200 || mc.AccessToken == "" {
		return nil, fmt.Errorf("Minecraft services refused the sign-in (HTTP %d)", st)
	}

	var ent struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if _, err := c.get(ctx, c.EP.Entitlements, mc.AccessToken, &ent); err != nil {
		return nil, err
	}
	owns := false
	for _, it := range ent.Items {
		if it.Name == "game_minecraft" || it.Name == "product_minecraft" {
			owns = true
		}
	}
	if !owns {
		return nil, ErrNoOwner
	}

	var prof struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	st, err = c.get(ctx, c.EP.Profile, mc.AccessToken, &prof)
	if err != nil {
		return nil, err
	}
	if st == 404 || prof.ID == "" {
		return nil, ErrNoProfile
	}
	return &Session{AccessToken: mc.AccessToken, Expires: time.Now().Add(time.Duration(mc.ExpiresIn) * time.Second),
		UUID: strings.ReplaceAll(prof.ID, "-", ""), Name: prof.Name}, nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
