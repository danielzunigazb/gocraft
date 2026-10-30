package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Stored is what is kept between runs: the long-lived Microsoft refresh token and who it is for. The
// Minecraft access token (24 hours) is never written down. The file is private to the user (0600), but
// it is a file: Horizon keeps this in the system keychain, which gocraft has no way to reach yet.
type Stored struct {
	RefreshToken string `json:"refresh_token"`
	UUID         string `json:"uuid"`
	Name         string `json:"name"`
}

// Path is where the account lives: <config dir>/gocraft/account.json.
func Path(configDir string) string { return filepath.Join(configDir, "gocraft", "account.json") }

// Load reads the saved account; a missing file is not an error.
func Load(path string) (*Stored, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Stored
	if err := json.Unmarshal(b, &s); err != nil || s.RefreshToken == "" {
		return nil, nil
	}
	return &s, nil
}

// Save writes the account atomically, readable only by the user.
func Save(path string, s *Stored) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(s)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Delete signs out: the saved account is removed.
func Delete(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
