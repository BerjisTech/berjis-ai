package main

import (
	"log"
	"os"

	"github.com/berjistech/berjis-ecosystem/ai/service/internal/config"
	dbi "github.com/berjistech/berjis-ecosystem/ai/service/internal/db"
	"github.com/berjistech/berjis-ecosystem/ai/service/internal/server"
)

func main() {
	cfg := config.Load()
	var dbc *dbi.DB
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		if db, err := dbi.Connect(dsn); err != nil {
			log.Printf("warn: ai-db connect failed: %v", err)
		} else {
			dbc = db
			if err := dbi.Setup(db); err != nil {
				log.Printf("warn: ai-db setup failed: %v", err)
			}
		}
	}
	app := server.New(server.Options{Config: cfg, DB: dbc})

	addr := ":" + cfg.Port
	log.Printf("starting berjis-ai on %s (env=%s)", addr, cfg.Env)
	if err := app.Listen(addr); err != nil {
		log.Println("shutdown:", err)
		os.Exit(1)
	}
}
