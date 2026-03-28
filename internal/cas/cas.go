package cas

import (
	"crypto/md5"
	"fmt"
	"os"
)

func Stash(data string, objPath string) (string, error) {

	// Create hash of data.
	hash := md5.Sum([]byte(data))

	hexHash := fmt.Sprintf("%x", hash)

	// Create directory.

	// Shard name is first 2 hex chars (1 byte).
	shardName := hexHash[0:2]
	// File name is remaining 30 hex chars.
	fileName := hexHash[2:]

	// Create directory.
	path := objPath + "/" + shardName
	fmt.Println("Path: " + path)

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

	n, e := f.WriteString(data)
	if e != nil {
		return "", e
	}

	return hexHash, nil

}

func Fetch(hexHash string, objPath string) (string, error) {

	// Shard name is first 2 hex chars (1 byte).
	shardName := hexHash[0:2]
	// File name is remaining 30 hex chars.
	fileName := hexHash[2:]

	filePath := objPath + "/" + shardName + "/" + fileName

	// Read file.
	data, e := os.ReadFile(filePath)
	if e != nil {
		return "", e
	}

	return string(data), nil
}
