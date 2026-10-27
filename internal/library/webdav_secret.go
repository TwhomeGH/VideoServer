package library

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/TwhomeGH/VideoServer/internal/atomicfile"
)

type webDAVSecret struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Store) webDAVSecretsPath() string {
	return filepath.Join(s.dataDirectory, "webdav-secrets.json")
}

func (s *Store) readWebDAVSecrets() (map[string]webDAVSecret, error) {
	secrets := map[string]webDAVSecret{}
	data, err := os.ReadFile(s.webDAVSecretsPath())
	if os.IsNotExist(err) {
		return secrets, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &secrets); err != nil {
		return nil, err
	}
	return secrets, nil
}

func (s *Store) saveWebDAVCredentials(poolID, username, password string) error {
	s.secretMu.Lock()
	defer s.secretMu.Unlock()
	secrets, err := s.readWebDAVSecrets()
	if err != nil {
		return err
	}
	secrets[poolID] = webDAVSecret{Username: username, Password: password}
	return atomicfile.WriteJSON(s.webDAVSecretsPath(), secrets)
}

func (s *Store) WebDAVCredentials(poolID string) (string, string, error) {
	s.secretMu.Lock()
	defer s.secretMu.Unlock()
	secrets, err := s.readWebDAVSecrets()
	if err != nil {
		return "", "", err
	}
	secret, exists := secrets[poolID]
	if !exists {
		return "", "", os.ErrNotExist
	}
	return secret.Username, secret.Password, nil
}
