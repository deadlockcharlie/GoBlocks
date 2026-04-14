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
	ZKAddress string
	ZKPort    string
	Name      string
	Host      string
	Port      string
}

func Load() *Config {
	port := flag.String("port", "3000", "Port to listen on")
	flag.Parse()

	//replicas := os.Getenv("REPLICAS")
	//role := os.Getenv("ROLE")
	zkAddress := os.Getenv("ZKAddress")
	name := os.Getenv("Name")
	zkPort := os.Getenv("ZKPort")

	cfg := &Config{
		Port: *port,
		//Role:      RoleSecondary,
		ZKAddress: zkAddress,
		ZKPort:    zkPort,
		Name:      name,
	}

	return cfg
}
