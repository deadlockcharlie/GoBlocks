package server

import (
	"log"
	"net/http"

	"blockstore/config"
	"blockstore/replication"
	"blockstore/storage"
)

type Server struct {
	handler *Handler
	port    string
}

func New(cfg *config.Config) *Server {
	store := storage.New()

	var replClient *replication.Client
	isPrimary := cfg.Role == config.RolePrimary

	if isPrimary && len(cfg.Replicas) > 0 {
		replClient = replication.NewClient(cfg.Replicas)
		log.Printf("Primary mode - discovered replicas: %v", cfg.Replicas)
	}

	handler := NewHandler(store, replClient, isPrimary)

	return &Server{
		handler: handler,
		port:    cfg.Port,
	}
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
