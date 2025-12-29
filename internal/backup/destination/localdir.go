package destination

import (
	"fmt"
	"os"
	"path/filepath"
)

type LocalDirConfig struct {
	Path string
}

type LocalDir struct {
	conf LocalDirConfig
}

func NewLocalDir(conf LocalDirConfig) (Interface, error) {
	return &LocalDir{conf: conf}, nil
}

func (d *LocalDir) getLocalPath(relPath string) string {
	return filepath.Join(d.conf.Path, relPath)
}

func (d *LocalDir) ReadDir(relPath string) ([]FileInfo, error) {
	files, err := os.ReadDir(d.getLocalPath(relPath))
	if err != nil {
		return nil, err
	}

	fis := make([]FileInfo, 0, len(files))
	for _, fi := range files {
		if fi.Name() != "." && fi.Name() != ".." {
			fis = append(fis, FileInfo{
				Name:  fi.Name(),
				IsDir: fi.IsDir(),
			})
		}
	}
	return fis, nil
}

func (d *LocalDir) FileExists(relPath string) (bool, error) {
	fi, err := os.Stat(d.getLocalPath(relPath))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if fi.IsDir() {
		return false, fmt.Errorf("expected file, but %q is a directory", relPath)
	}
	return true, nil
}

func (d *LocalDir) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(d.getLocalPath(relPath))
}

func (d *LocalDir) WriteFile(relPath string, data []byte) error {
	file := d.getLocalPath(relPath)
	if err := os.MkdirAll(filepath.Dir(file), os.ModePerm); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	return os.WriteFile(file, data, os.ModePerm)
}

func (d *LocalDir) DeleteDir(relPath string) error {
	return os.RemoveAll(d.getLocalPath(relPath))
}

func (d *LocalDir) CreateDir(relPath string) error {
	return os.MkdirAll(d.getLocalPath(relPath), os.ModePerm)
}

func (d *LocalDir) IsNotExists(err error) bool {
	return os.IsNotExist(err)
}
