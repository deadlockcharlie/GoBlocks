package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"blockstore/config"
	"blockstore/server"
)

func main() {
	cfg := config.Load()

	log.Printf("Starting block store.")

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("Couldn't initialize the server. Reason: %s", err)

	}

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	log.Println("Use ctrl+c to stop the server.")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-quit
	srv.Shutdown()
	log.Println("Shutdown complete")
}
