package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

// Config holds all server configuration loaded from environment variables.
// Path and credential values are required; operational values have defaults
// that can be overridden.
type Config struct {
	// Required — server fails to start if these are missing.
	ObjPath    string // HATCHECK_DATA
	MetaPath   string // HATCHECK_META
	UIPath     string // HATCHECK_UI
	SigningKey []byte // HATCHECK_SIGNING_KEY

	// Optional — identity and auth.
	BootstrapToken string // HATCHECK_BOOTSTRAP_TOKEN

	// Operational — tunable without changing authorization semantics.
	CapabilityExpiry time.Duration // HATCHECK_CAPABILITY_EXPIRY (default: 8760h)

	RateReadTokens  rate.Limit // HATCHECK_RATE_READ_INTERVAL (default: 1s)
	RateReadBurst   int        // HATCHECK_RATE_READ_BURST    (default: 10)
	RateWriteTokens rate.Limit // HATCHECK_RATE_WRITE_INTERVAL (default: 5s)
	RateWriteBurst  int        // HATCHECK_RATE_WRITE_BURST   (default: 4)
	RateAdminTokens rate.Limit // HATCHECK_RATE_ADMIN_INTERVAL (default: 30s)
	RateAdminBurst  int        // HATCHECK_RATE_ADMIN_BURST   (default: 2)
}

// LoadConfig reads configuration from environment variables and returns a
// populated Config. It logs all resolved values at startup (excluding secrets)
// so the running configuration is visible in the server log.
func LoadConfig() Config {
	cfg := Config{
		// Paths with defaults.
		ObjPath:  envOr("HATCHECK_DATA", "./objects"),
		MetaPath: envOr("HATCHECK_META", "./metadata"),
		UIPath:   envOr("HATCHECK_UI", "./ui"),

		// Optional.
		BootstrapToken: os.Getenv("HATCHECK_BOOTSTRAP_TOKEN"),

		// Operational defaults.
		CapabilityExpiry: envDuration("HATCHECK_CAPABILITY_EXPIRY", 365*24*time.Hour),

		RateReadTokens:  rate.Every(envDuration("HATCHECK_RATE_READ_INTERVAL", time.Second)),
		RateReadBurst:   envInt("HATCHECK_RATE_READ_BURST", 10),
		RateWriteTokens: rate.Every(envDuration("HATCHECK_RATE_WRITE_INTERVAL", 5*time.Second)),
		RateWriteBurst:  envInt("HATCHECK_RATE_WRITE_BURST", 4),
		RateAdminTokens: rate.Every(envDuration("HATCHECK_RATE_ADMIN_INTERVAL", 30*time.Second)),
		RateAdminBurst:  envInt("HATCHECK_RATE_ADMIN_BURST", 2),
	}

	// Required — signing key must be set.
	signingKey := os.Getenv("HATCHECK_SIGNING_KEY")
	if signingKey == "" {
		log.Fatal("HATCHECK_SIGNING_KEY environment variable must be set")
	}
	cfg.SigningKey = []byte(signingKey)

	// Log resolved configuration (secrets omitted).
	log.Printf("config: data=%s meta=%s ui=%s", cfg.ObjPath, cfg.MetaPath, cfg.UIPath)
	log.Printf("config: capability_expiry=%s", cfg.CapabilityExpiry)
	log.Printf("config: rate read_burst=%d write_burst=%d admin_burst=%d",
		cfg.RateReadBurst, cfg.RateWriteBurst, cfg.RateAdminBurst)
	if cfg.BootstrapToken != "" {
		log.Printf("config: bootstrap token set — disable after initial setup")
	}

	return cfg
}

// envOr returns the value of the named environment variable, or the default
// if the variable is unset or empty.
func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// envInt returns the integer value of the named environment variable, or the
// default if the variable is unset, empty, or unparseable.
func envInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("warning: %s=%q is not a valid integer, using default %d", key, v, defaultVal)
		return defaultVal
	}
	return n
}

// envDuration returns the duration value of the named environment variable,
// or the default if the variable is unset, empty, or unparseable.
// Values must be in Go duration format, e.g. "8760h", "5s", "30m".
func envDuration(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("warning: %s=%q is not a valid duration, using default %s", key, v, defaultVal)
		return defaultVal
	}
	return d
}

// String returns a human-readable description of the Config for debugging.
func (c Config) String() string {
	return fmt.Sprintf(
		"Config{ObjPath:%s MetaPath:%s UIPath:%s CapabilityExpiry:%s "+
			"RateReadBurst:%d RateWriteBurst:%d RateAdminBurst:%d}",
		c.ObjPath, c.MetaPath, c.UIPath, c.CapabilityExpiry,
		c.RateReadBurst, c.RateWriteBurst, c.RateAdminBurst,
	)
}
