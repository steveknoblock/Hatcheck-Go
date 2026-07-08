package main

import (
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/steveknoblock/hatcheck-go/internal/cas"
	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// rateLimitInfo describes one rate-limit pool in human-readable form.
type rateLimitInfo struct {
	Interval string `json:"interval"` // e.g. "1s", "30s"
	Burst    int    `json:"burst"`
}

// configResponse is the non-secret subset of Config, plus a few cheap
// store statistics. Never include SigningKey or the BootstrapToken value —
// only whether a bootstrap token is set, since that's operationally useful
// (a lingering bootstrap token is meant to be rotated out once real admin
// capabilities exist) without exposing the secret itself.
type configResponse struct {
	ObjPath           string        `json:"obj_path"`
	MetaPath          string        `json:"meta_path"`
	UIPath            string        `json:"ui_path"`
	CapabilityExpiry  string        `json:"capability_expiry"`
	BootstrapTokenSet bool          `json:"bootstrap_token_set"`
	RateLimits        rateLimitsMap `json:"rate_limits"`
	Stats             configStats   `json:"stats"`
}

type rateLimitsMap struct {
	Read  rateLimitInfo `json:"read"`
	Write rateLimitInfo `json:"write"`
	Admin rateLimitInfo `json:"admin"`
}

type configStats struct {
	ObjectCount     int `json:"object_count"`
	CapabilityCount int `json:"capability_count"`
	PrincipalCount  int `json:"principal_count"`
	TagCount        int `json:"tag_count"`
	NamespaceCount  int `json:"namespace_count"`
}

// rateLimitIntervalString reconstructs a human-readable interval from a
// rate.Limit (stored as events/second). rate.Every(d) computes
// Limit(1/d.Seconds()), so this inverts that to recover an approximate
// duration for display purposes only — not re-parsed back into Config.
func rateLimitIntervalString(l rate.Limit) string {
	if l <= 0 {
		return "unlimited"
	}
	d := time.Duration(float64(time.Second) / float64(l))
	return d.String()
}

// configHandler returns non-secret configuration and basic store
// statistics for admin visibility. Only principals with PermAdmin may call
// this — same cm.Protect(PermAdmin, ...) wrapping as the other admin routes.
//
// GET /config
func configHandler(w http.ResponseWriter, req *http.Request, cfg Config, store *cas.Store, meta *metadata.Store, vr VerifiedRequest) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	objects, err := store.List()
	if err != nil {
		http.Error(w, "failed to list objects", http.StatusInternalServerError)
		return
	}

	resp := configResponse{
		ObjPath:           cfg.ObjPath,
		MetaPath:          cfg.MetaPath,
		UIPath:            cfg.UIPath,
		CapabilityExpiry:  cfg.CapabilityExpiry.String(),
		BootstrapTokenSet: cfg.BootstrapToken != "",
		RateLimits: rateLimitsMap{
			Read:  rateLimitInfo{Interval: rateLimitIntervalString(cfg.RateReadTokens), Burst: cfg.RateReadBurst},
			Write: rateLimitInfo{Interval: rateLimitIntervalString(cfg.RateWriteTokens), Burst: cfg.RateWriteBurst},
			Admin: rateLimitInfo{Interval: rateLimitIntervalString(cfg.RateAdminTokens), Burst: cfg.RateAdminBurst},
		},
		Stats: configStats{
			ObjectCount:     len(objects),
			CapabilityCount: len(meta.AllCapabilities()),
			PrincipalCount:  len(meta.Principals()),
			TagCount:        len(meta.AllTags()),
			NamespaceCount:  len(meta.Namespaces()),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
