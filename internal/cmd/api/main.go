package main

import (
	"api-library/internal/app"
	"api-library/internal/config"
	"api-library/internal/infrastructure"
	"api-library/internal/route"
	"encoding/json"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v3"
)

func main() {

	appCfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("LoadConfig err: %+v", err)
	}
	jsonBytes, _ := json.MarshalIndent(appCfg, "", "  ")
	log.Printf("Loaded appCfg %+v", string(jsonBytes))

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

	container := app.NewContainer(
		cache,
		db,
		dbRo,
		esClient,
	)
	defer container.Close()
	log.Printf("container %+v", container)

	// 建立 Web 框架實例與註冊路由
	fiberApp := fiber.New()
	route.RegisterRoutes(fiberApp, appCfg, container)

	// 啟動 Server
	addr := fmt.Sprintf(":%d", appCfg.App.Port)
	if err := fiberApp.Listen(addr); err != nil {
		log.Fatalf("[BOOT] Server failed to start: %v", err)
	}
}
