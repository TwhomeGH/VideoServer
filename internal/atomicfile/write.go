// Package atomicfile 提供同目錄暫存後替換的檔案保存，避免直接截斷目標檔案。
package atomicfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteJSON 將 value 編碼後完整寫入同目錄暫存檔，Sync、關閉後替換 path。
// 替換前失敗會保留舊檔；失敗的暫存檔保留供診斷，不進行額外清理。
// 替換後目錄同步失敗會明確回傳「已替換」錯誤，不聲稱已回復舊檔。
// path 的父目錄必須存在；首次建立使用 0600，既有檔案沿用 FileMode 權限位元。
func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return write(path, data, replace)
}

func write(path string, data []byte, commit func(string, string) error) error {
	mode := os.FileMode(0600)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("目標不是一般檔案：%s", path)
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".pending-*")
	if err != nil {
		return err
	}
	pending := file.Name()
	closed := false
	defer func() {
		if !closed {
			file.Close()
		}
	}()
	fail := func(err error) error {
		return fmt.Errorf("保存失敗，原檔未替換；暫存檔保留於 %s：%w", pending, err)
	}
	if err := file.Chmod(mode); err != nil {
		return fail(err)
	}
	if _, err := file.Write(data); err != nil {
		return fail(err)
	}
	if err := file.Sync(); err != nil {
		return fail(err)
	}
	if err := file.Close(); err != nil {
		closed = true
		return fail(err)
	}
	closed = true
	if err := commit(pending, path); err != nil {
		return fail(err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("檔案已替換，但同步目錄失敗：%w", err)
	}
	return nil
}
