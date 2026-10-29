package opencode

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// copyFile copies src to dst. Returns true if the file was actually written.
func copyFile(src, dst string) (changed bool, err error) {
	srcData, err := os.ReadFile(src)
	if err != nil {
		return false, err
	}
	if dstData, err := os.ReadFile(dst); err == nil {
		if sha256hex(srcData) == sha256hex(dstData) {
			return false, nil // already identical
		}
	}
	return true, os.WriteFile(dst, srcData, 0o644)
}

func filesEqual(a, b string) (bool, error) {
	da, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return sha256hex(da) == sha256hex(db), nil
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// prettyPath shortens the home directory to ~ for display.
func prettyPath(path string) string {
	home, _ := os.UserHomeDir()
	if home != "" {
		if rel, err := filepath.Rel(home, path); err == nil {
			return "~/" + rel
		}
	}
	return path
}
