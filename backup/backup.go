package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const FormatVersion = 1

type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Version int    `json:"version"`
	Files   []File `json:"files"`
}

func Create(dataDir, destination string) error {
	if err := os.Mkdir(destination, 0o700); err != nil {
		return err
	}
	manifest := Manifest{Version: FormatVersion}
	for _, name := range []string{"raft.wal", "snapshot.bin"} {
		source := filepath.Join(dataDir, name)
		if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		entry, err := copyVerified(source, filepath.Join(destination, name))
		if err != nil {
			return err
		}
		entry.Name = name
		manifest.Files = append(manifest.Files, entry)
	}
	if len(manifest.Files) == 0 {
		return errors.New("backup: no node data found")
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(destination, "manifest.json"), data, 0o600)
}

func Restore(source, dataDir string) error {
	entries, err := os.ReadDir(dataDir)
	if err == nil && len(entries) != 0 {
		return errors.New("backup: restore destination is not empty")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := os.ReadFile(filepath.Join(source, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Version != FormatVersion {
		return errors.New("backup: unsupported manifest")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	for _, entry := range manifest.Files {
		if entry.Name != "raft.wal" && entry.Name != "snapshot.bin" {
			return errors.New("backup: invalid file name")
		}
		copied, err := copyVerified(filepath.Join(source, entry.Name), filepath.Join(dataDir, entry.Name))
		if err != nil {
			return err
		}
		if copied.Size != entry.Size || copied.SHA256 != entry.SHA256 {
			return errors.New("backup: checksum mismatch")
		}
	}
	return nil
}

func copyVerified(source, destination string) (File, error) {
	input, err := os.Open(source)
	if err != nil {
		return File{}, err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return File{}, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return File{}, copyErr
	}
	if syncErr != nil {
		return File{}, syncErr
	}
	if closeErr != nil {
		return File{}, closeErr
	}
	return File{Size: size, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}
