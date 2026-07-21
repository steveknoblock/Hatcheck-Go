package cas

import (
	"fmt"
	"os"
	"path/filepath"
)

const shardLen = 2

type HashFunc func(content string) string

type Store struct {
	objPath  string
	hashFunc HashFunc
}

func New(objPath string, hashFunc HashFunc) (*Store, error) {
	if hashFunc == nil {
		return nil, fmt.Errorf("hashFunc must not be nil")
	}
	if err := os.MkdirAll(objPath, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create object store at %q: %w", objPath, err)
	}
	return &Store{objPath: objPath, hashFunc: hashFunc}, nil
}

// Stash stores content in the CAS and returns its hash. Stashing the same
// content twice is a no-op the second time (the file already exists) and
// returns the same hash, since the hash is derived purely from content.
func (store *Store) Stash(content string) (string, error) {
	hash := store.hashFunc(content)
	if len(hash) < 3 {
		return "", fmt.Errorf("hash too short: %q", hash)
	}

	// Shard name is the first 2 hex chars, file name is the rest — keeps
	// any single directory from accumulating too many entries as the
	// store grows.
	shardName := hash[0:shardLen]
	fileName := hash[shardLen:]
	shardPath := filepath.Join(store.objPath, shardName)

	if err := os.MkdirAll(shardPath, 0755); err != nil {
		return "", err
	}

	f, err := os.Create(filepath.Join(shardPath, fileName))
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return "", err
	}

	return hash, nil
}

// Fetch retrieves content by hash.
func (store *Store) Fetch(hash string) (string, error) {
	if len(hash) < 3 {
		return "", fmt.Errorf("hash too short: %q", hash)
	}

	shardName := hash[0:shardLen]
	fileName := hash[shardLen:]
	filePath := filepath.Join(store.objPath, shardName, fileName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// Walk directory to build list of hashes.
func (store *Store) List() ([]string, error) {
	var hashes []string
	err := filepath.Walk(store.objPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			shard := filepath.Base(filepath.Dir(path))
			file := filepath.Base(path)
			hashes = append(hashes, shard+file)
		}
		return nil
	})
	return hashes, err
}
