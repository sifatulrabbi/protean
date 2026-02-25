package main

import (
	"log"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/protean/vfs-server/internal/config"
	"github.com/protean/vfs-server/internal/handler"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	router := handler.NewRouter(cfg.WorkspaceBase, cfg.ServiceTokens)

	// Wrap with Recovery and Logger at the outermost level
	outerHandler := chimw.Recoverer(chimw.Logger(router))

	log.Printf("vfs-server listening on :%s (workspace=%s)", cfg.Port, cfg.WorkspaceBase)
	if err := http.ListenAndServe(":"+cfg.Port, outerHandler); err != nil {
		log.Fatalf("server: %v", err)
	}
}
