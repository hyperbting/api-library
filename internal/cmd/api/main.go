package main

import (
	"api-library/internal/app"
	"api-library/internal/config"
	"api-library/internal/infrastructure"
	"api-library/internal/route"
	"api-library/pkg/session"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v3"
)

func main() {

	// load env from config.yaml then config.local.yaml
	appCfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("LoadConfig err: %+v", err)
	}
	_ = appCfg.DebugPrint()

	// init infrastructures
	cache, err := infrastructure.NewRedis(appCfg.Redis)
	if err != nil {
		log.Fatalf("Cache init failed: %v", err)
	}

	db, err := infrastructure.NewPostgreSQL(&appCfg.Database.MasterDB)
	if err != nil {
		log.Fatalf("Master DB init failed: %v", err)
	}

	dbRo, err := infrastructure.NewPostgreSQL(&appCfg.Database.ReadOnlyDB)
	if err != nil {
		log.Fatalf("RO DB init failed: %v", err)
	}

	esClient, err := infrastructure.NewElasticsearch(appCfg.Elastic)
	if err != nil {
		log.Fatalf("Cronjob ES init failed: %v", err)
	}

	tknManager, err := session.NewTokenManager(appCfg.SessionJWT)
	if err != nil {
		log.Fatalf("Session Manager init failed: %v", err)
	}

	// Create container
	container := app.NewContainer(
		cache,
		db,
		dbRo,
		esClient,
		tknManager,
	)
	defer container.Close()
	log.Printf("container %+v", container)

	// init fiber app and register routes
	fiberApp := fiber.New()
	route.RegisterRoutes(fiberApp, appCfg, container)

	// start server
	addr := fmt.Sprintf(":%d", appCfg.App.Port)
	if err := fiberApp.Listen(addr); err != nil {
		log.Fatalf("[BOOT] Server failed to start: %v", err)
	}
}
