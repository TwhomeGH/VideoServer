package auth

import (
	"fmt"
	"os"
	"path/filepath"
)

const initialPasswordFile = "initial-password.txt"

// WriteInitialPassword 將一次性密碼寫入資料目錄的 0600 檔案；回傳檔案路徑。
func WriteInitialPassword(dataDir, password string) (string, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dataDir, initialPasswordFile)
	pending, err := os.CreateTemp(dataDir, "."+initialPasswordFile+".pending-*")
	if err != nil {
		return "", err
	}
	tmp := pending.Name()
	cleanup := true
	defer func() {
		pending.Close()
		if cleanup {
			os.Remove(tmp)
		}
	}()
	if err := pending.Chmod(0600); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(pending, "%s\n", password); err != nil {
		return "", err
	}
	if err := pending.Close(); err != nil {
		return "", err
	}
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	cleanup = false
	return path, nil
}

func (m *Manager) clearInitialPassword() {
	if m.initialPasswordPath == "" {
		return
	}
	_ = os.Remove(m.initialPasswordPath)
}
