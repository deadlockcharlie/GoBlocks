package config

import (
	"flag"
	"os"
	"strings"
)

const (
	BlockSize     = 4096
	RolePrimary   = "primary"
	RoleSecondary = "secondary"
)

type Config struct {
	Port     string
	Role     string
	Replicas []string
}

func Load() *Config {
	port := flag.String("port", "8080", "Port to listen on")
	flag.Parse()

	replicas := os.Getenv("REPLICAS")
	role := os.Getenv("ROLE")

	cfg := &Config{
		Port: *port,
		Role: RoleSecondary,
	}

	if role == RolePrimary {
		cfg.Role = role
		if replicas != "" {
			cfg.Replicas = strings.Split(replicas, ",")
		}
	}

	return cfg
}
