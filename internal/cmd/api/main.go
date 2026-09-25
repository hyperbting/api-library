package main

import (
	"api-library/internal/app"
	"api-library/internal/config"
	"api-library/internal/route"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v3"
)

func main() {

	// load env from config.yaml then config.local.yaml
	appCfgP, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("LoadConfig err: %+v", err)
	}
	_ = appCfgP.DebugPrint()

	// Create container
	container, err := app.NewContainer(appCfgP)
	if err != nil {
		log.Fatalf("NewContainer err: %+v", err)
	}
	defer container.Close()
	log.Printf("container %+v", container)

	// init fiber app and register routes
	fiberApp := fiber.New()
	route.RegisterRoutes(fiberApp, appCfgP, container)

	// start server
	addr := fmt.Sprintf(":%d", appCfgP.App.Port)
	if err := fiberApp.Listen(addr); err != nil {
		log.Fatalf("[BOOT] Server failed to start: %v", err)
	}
}
