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
	Handler *Handler
	Port    string
	Node    *replication.Node
}

func New(cfg *config.Config) (*Server, error) {
	// Create a new store
	store := storage.New()
	node, err := replication.NewZookeeper(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create replication node: %v", err)
	}
	log.Printf("connected to zookeeper %s:%s", cfg.ZKAddress, cfg.ZKPort)
	if node.Replicas == nil || len(node.Replicas) == 0 {
		log.Print("Did not find any replicas in the ring.")
	}
	log.Printf("discovered Nodes: %v", node.Replicas)

	replClient := replication.NewClient(node)

	handler := NewHandler(store, replClient)
	return &Server{
		Handler: handler,
		Port:    cfg.ReplicaPort,
		Node:    node,
	}, nil
}

func (s *Server) Start() error {
	http.HandleFunc("/health", s.Handler.Health)

	http.HandleFunc("/block/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.Handler.GetBlock(w, r)
		case http.MethodPut:
			s.Handler.PutBlock(w, r)
		case http.MethodDelete:
			s.Handler.DeleteBlock(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/internal/block/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			s.Handler.InternalPutBlock(w, r)
		case http.MethodDelete:
			s.Handler.InternalDeleteBlock(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	addr := ":" + s.Port
	log.Printf("Server starting on %s", addr)
	return http.ListenAndServe(addr, nil)
}

func (s *Server) GetPort() string {
	return s.Port
}

func (s *Server) Shutdown() {
	// Capture user issues terminates via ctrl+c and ctrl+d and shutdown the server
	log.Println("Shutting down server...")
	s.Node.Shutdown()
}
