package config

import (
	"flag"
	"os"
)

const (
	BlockSize = 4096
	//RolePrimary   = "primary"
	//RoleSecondary = "secondary"
)

type Config struct {
	ZKAddress      string
	ZKPort         string
	ReplicaName    string
	ReplicaAddress string
	ReplicaPort    string
}

func Load() *Config {
	port := flag.String("port", "3000", "Port to listen on")
	flag.Parse()

	//replicas := os.Getenv("REPLICAS")
	//role := os.Getenv("ROLE")
	zkAddress := os.Getenv("ZKAddress")
	name := os.Getenv("ReplicaName")
	zkPort := os.Getenv("ZKPort")
	host := os.Getenv("ReplicaAddress")

	cfg := &Config{
		ReplicaPort: *port,
		//Role:      RoleSecondary,
		ZKAddress:      zkAddress,
		ZKPort:         zkPort,
		ReplicaName:    name,
		ReplicaAddress: host,
	}

	return cfg
}
