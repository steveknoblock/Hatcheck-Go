package cas

import (
	"fmt"
	"os"
)

const shardLen = 2

type HashFunc func(content string) string

type Store struct {
	objPath  string
	hashFunc HashFunc
}

func New(objPath string, hashFunc HashFunc) *Store {
	return &Store{objPath: objPath, hashFunc: hashFunc}
}

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

	// Create directory.
	path := store.objPath + "/" + shardName
	// fmt.Println("Path: " + path)

	if e := os.MkdirAll(path, os.ModePerm); e != nil {
		return "", e
	}

	// Create file.
	filePath := path + "/" + fileName
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

func (store *Store) Fetch(hash string) (string, error) {

	// Check hash is long enough to create shard and file name.
	if len(hash) < 3 {
		return "", fmt.Errorf("hash too short: %q", hash)
	}

	// Shard name is first 2 hex chars (1 byte).
	shardName := hash[0:shardLen]
	// File name is remaining hex chars.
	fileName := hash[shardLen:]

	filePath := store.objPath + "/" + shardName + "/" + fileName

	// Read file.
	data, e := os.ReadFile(filePath)
	if e != nil {
		return "", e
	}

	return string(data), nil
}
