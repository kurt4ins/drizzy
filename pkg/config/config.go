package config

import "os"

// Get returns the env var value or panics if not set.
func Get(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("required environment variable not set: " + key)
	}
	return v
}
