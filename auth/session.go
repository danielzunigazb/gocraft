package auth

import (
	"context"
	"crypto/sha1"
	"fmt"
	"math/big"
	"strings"
)

// ServerHash is the digest Minecraft sends to the session server when joining an online-mode server:
// the SHA-1 of the server ID, the shared secret and the server's public key, read as a signed
// two's-complement number and written in hex (a minus sign in front when it is negative).
func ServerHash(serverID string, secret, publicKey []byte) string {
	h := sha1.New()
	h.Write([]byte(serverID))
	h.Write(secret)
	h.Write(publicKey)
	sum := h.Sum(nil)
	n := new(big.Int).SetBytes(sum)
	if sum[0]&0x80 != 0 { // negative in two's complement: n - 2^160
		n.Sub(n, new(big.Int).Lsh(big.NewInt(1), 160))
	}
	return n.Text(16)
}

// Join tells the session server this player is joining the server with the given hash. An online-mode
// server then asks the same server whether the player did; that is how it knows who they are.
func (c *Client) Join(ctx context.Context, s *Session, serverHash string) error {
	st, err := c.post(ctx, c.EP.SessionJoin, nil, map[string]string{
		"accessToken": s.AccessToken, "selectedProfile": s.UUID, "serverId": serverHash,
	}, nil, nil)
	if err != nil {
		return err
	}
	if st != 204 && st != 200 {
		return fmt.Errorf("the session server refused the join (HTTP %d): sign in again", st)
	}
	return nil
}

// Dashed is the UUID with its dashes, as servers print it.
func Dashed(id string) string {
	id = strings.ReplaceAll(id, "-", "")
	if len(id) != 32 {
		return id
	}
	return id[0:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
}
