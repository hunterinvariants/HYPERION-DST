package snapshot

import (
	"errors"
	"os"
	"path/filepath"
)

var ErrRegression = errors.New("snapshot: index regression")

// Store publishes checksummed snapshot images using write-sync-rename-sync.
// A crash exposes either the previous complete image or the replacement.
type Store struct {
	path string
	last uint64
}

func OpenStore(path string) (*Store, Image, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, Image{}, nil
	}
	if err != nil {
		return nil, Image{}, err
	}
	image, err := Decode(data)
	if err != nil {
		return nil, Image{}, err
	}
	s.last = image.LastIndex
	return s, image, nil
}

func (s *Store) Save(image Image) error {
	if image.LastIndex < s.last {
		return ErrRegression
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(Encode(image))
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpName, s.path)
	}
	if err != nil {
		return err
	}
	if err := syncDirectory(dir); err != nil {
		return err
	}

	s.last, ok = image.LastIndex, true
	return nil
}
