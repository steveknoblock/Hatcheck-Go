package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

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

	hash, err := cas.Stash(string(body), string(objPath))
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

func main() {

	objPath := os.Getenv("HATCHECK_DATA")
	if objPath == "" {
		objPath = "../objects" // default if not set
	}

	http.HandleFunc("/stash", func(w http.ResponseWriter, req *http.Request) {
		stashHandler(w, req, objPath)
	})
	http.HandleFunc("/fetch", func(w http.ResponseWriter, req *http.Request) {
		fetchHandler(w, req, objPath)
	})

	log.Println("starting server on :8090")
	if err := http.ListenAndServe(":8090", nil); err != nil {
		log.Fatal(err)
	}
}
