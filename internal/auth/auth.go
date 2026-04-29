// package auth
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/login", sendLoginHandler)
	http.HandleFunc("/authenticate", authenticateHandler)
	http.HandleFunc("/dashboard", dashboardHandler)
	http.ListenAndServe(":3000", nil)
}

// Implement sendLoginHandler/authenticateHandler/dashboardHandler using the
// requests shown above.
func sendLoginHandler(w http.ResponseWriter, r *http.Request) {
	payload := map[string]any{
		"email":                 "steve.knoblock@gmail.com",
		"login_magic_link_url":  "http://localhost:3000/authenticate",
		"signup_magic_link_url": "http://localhost:3000/authenticate",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "https://test.stytch.com/v1/magic_links/email/login_or_create", bytes.NewReader(body))
	req.SetBasicAuth(os.Getenv("STYTCH_PROJECT_ID"), os.Getenv("STYTCH_SECRET"))
	req.Header.Set("Content-Type", "application/json")
}

func authenticateHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	payload := map[string]any{
		"token":                    token,
		"session_duration_minutes": 60,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "https://test.stytch.com/v1/magic_links/authenticate", bytes.NewReader(body))
	req.SetBasicAuth(os.Getenv("STYTCH_PROJECT_ID"), os.Getenv("STYTCH_SECRET"))
	req.Header.Set("Content-Type", "application/json")
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	sessionJWT := r.Header.Get("Authorization")
	payload := map[string]any{"session_jwt": sessionJWT}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "https://test.stytch.com/v1/sessions/authenticate", bytes.NewReader(body))
	req.SetBasicAuth(os.Getenv("STYTCH_PROJECT_ID"), os.Getenv("STYTCH_SECRET"))
	req.Header.Set("Content-Type", "application/json")
}
