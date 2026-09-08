package main

import (
	"log"
	"net/http"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/config"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/server"
)

func main() {
	cfg := config.MustLoad()
	handler := server.NewRouter(cfg)

	log.Printf("plugin service listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, handler); err != nil {
		log.Fatal(err)
	}
}
