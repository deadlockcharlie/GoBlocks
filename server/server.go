package server

import (
	"blockstore/config"
	"blockstore/replication"
	"blockstore/storage"
	"fmt"
	"log"
	"net/http"
)

type Server struct {
	handler *Handler
	port    string
	node    *replication.Node
}

func New(cfg *config.Config) (*Server, error) {
	// Create a new store
	store := storage.New()
	node, err := replication.New(cfg.Name, cfg.ZKAddress, cfg.ZKPort)
	if err != nil {
		return nil, fmt.Errorf("failed to create replication node: %v", err)
	}
	log.Printf("connected to zookeeper %s:%s", cfg.ZKAddress, cfg.ZKPort)
	if node.Replicas == nil || len(node.Replicas) == 0 {
		log.Print("Did not find any replicas in the ring.")
	}
	log.Printf("discovered Nodes: %v", node.Replicas)

	var replClient *replication.Client

	handler := NewHandler(store, replClient)
	return &Server{
		handler: handler,
		port:    cfg.Port,
		node:    node,
	}, nil
}

func (s *Server) Start() error {
	http.HandleFunc("/health", s.handler.Health)

	http.HandleFunc("/block/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handler.GetBlock(w, r)
		case http.MethodPut:
			s.handler.PutBlock(w, r)
		case http.MethodDelete:
			s.handler.DeleteBlock(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/internal/block/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			s.handler.InternalPutBlock(w, r)
		case http.MethodDelete:
			s.handler.InternalDeleteBlock(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	addr := ":" + s.port
	log.Printf("Server starting on %s", addr)
	return http.ListenAndServe(addr, nil)
}

func (s *Server) GetPort() string {
	return s.port
}

func (s *Server) Shutdown() {
	// Capture user issues terminates via ctrl+c and ctrl+d and shutdown the server
	log.Println("Shutting down server...")
	s.node.Shutdown()
}
