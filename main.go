package main

import (
	"log"

	"blockstore/config"
	"blockstore/server"
)

func main() {
	cfg := config.Load()

	log.Printf("Starting block store - Role: %s, Port: %s", cfg.Role, cfg.Port)

	srv := server.New(cfg)

	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
