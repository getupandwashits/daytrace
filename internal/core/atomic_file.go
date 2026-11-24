package core

import (
	"os"
	"path/filepath"
)

func syncDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, ".write-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(name)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
