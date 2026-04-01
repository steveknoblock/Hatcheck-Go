package cas

import (
	"fmt"
	"os"
	"path/filepath"
)

const shardLen = 2
const directorySeparator = "/"

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

// Stashes a value in the store.
func (store *Store) Stash(content string) (string, error) {

	// Create hash of content.

	// Hash content using provided hash function.
	hash := store.hashFunc(content)

	// Create shard and file name from hash.

	// Check hash is long enough to create shard and file name.
	if len(hash) < 3 {
		return "", fmt.Errorf("hash too short: %q", hash)
	}
	// Shard name is first 2 hex chars (1 byte).
	shardName := hash[0:shardLen]
	// File name is remaining hex chars.
	fileName := hash[shardLen:]

	// Create file.
	filePath := store.objPath + directorySeparator + shardName + directorySeparator + fileName
	f, e := os.Create(filePath)
	if e != nil {
		return "", e
	}
	defer f.Close()

	_, e = f.WriteString(content)
	if e != nil {
		return "", e
	}

	return hash, nil

}

// Fetches a value from the store.
func (store *Store) Fetch(hash string) (string, error) {

	// Check hash is long enough to create shard and file name.
	if len(hash) < 3 {
		return "", fmt.Errorf("hash too short: %q", hash)
	}

	// Shard name is first 2 hex chars (1 byte).
	shardName := hash[0:shardLen]
	// File name is remaining hex chars.
	fileName := hash[shardLen:]

	filePath := store.objPath + directorySeparator + shardName + directorySeparator + fileName

	// Read file.
	data, e := os.ReadFile(filePath)
	if e != nil {
		return "", e
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
