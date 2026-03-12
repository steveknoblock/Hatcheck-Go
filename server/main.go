package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/steveknoblock/hatcheck-go/cas"
)

func stashHandler(w http.ResponseWriter, req *http.Request, objPath string) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	hash, err := cas.Stash(string(body), objPath)
	if err != nil {
		http.Error(w, "failed to stash content", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "%s\n", hash)
}

func fetchHandler(w http.ResponseWriter, req *http.Request, objPath string) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hash := req.URL.Query().Get("hash")
	if hash == "" {
		http.Error(w, "missing hash parameter", http.StatusBadRequest)
		return
	}

	data, err := cas.Fetch(hash, objPath)
	if err != nil {
		http.Error(w, "content not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s\n", data)
}

func listHandler(w http.ResponseWriter, req *http.Request, objPath string) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var hashes []string

	err := filepath.Walk(objPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip directories, only process files
		if !info.IsDir() {
			// Reconstruct hash from shard directory name and filename
			shard := filepath.Base(filepath.Dir(path))
			file := filepath.Base(path)
			hashes = append(hashes, shard+file)
		}
		return nil
	})

	if err != nil {
		http.Error(w, "failed to list objects", http.StatusInternalServerError)
		return
	}

	if hashes == nil {
		hashes = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hashes)
}

func main() {
	objPath := os.Getenv("HATCHECK_DATA")
	if objPath == "" {
		objPath = "../objects" // default if not set
	}

	uiPath := os.Getenv("HATCHECK_UI")
	if uiPath == "" {
		uiPath = "../ui" // default if not set
	}

	http.HandleFunc("/stash", func(w http.ResponseWriter, req *http.Request) {
		stashHandler(w, req, objPath)
	})
	http.HandleFunc("/fetch", func(w http.ResponseWriter, req *http.Request) {
		fetchHandler(w, req, objPath)
	})
	http.HandleFunc("/list", func(w http.ResponseWriter, req *http.Request) {
		listHandler(w, req, objPath)
	})

	// Serve static UI files
	http.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir(uiPath))))

	log.Println("starting server on :8090")
	if err := http.ListenAndServe(":8090", nil); err != nil {
		log.Fatal(err)
	}
}
