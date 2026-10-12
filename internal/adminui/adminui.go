// Package adminui serves the embedded admin panel (the Vue app built into dist/) and its runtime config.
package adminui

import (
	"api-library/internal/config"
	"embed"
	"io/fs"
	"mime"
	"path"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// dist holds the Vite build output (admin/ -> internal/adminui/dist). Only .gitkeep is committed;
// the Docker build fills it in. `all:` keeps dotfiles so the embed compiles with an empty dist.
//
//go:embed all:dist
var distFS embed.FS

const basePath = "/admin"

// PublicConfig is what the panel loads before sign-in. All values are public (the Firebase web config is
// shipped to browsers anyway); never put secrets here.
type PublicConfig struct {
	Login struct {
		Google bool `json:"google"`
		Email  bool `json:"email"`
	} `json:"login"`
	Firebase struct {
		APIKey     string `json:"apiKey"`
		AuthDomain string `json:"authDomain"`
		ProjectID  string `json:"projectId"`
		AppID      string `json:"appId"`
	} `json:"firebase"`
	// Register tells the panel to show its "create an account" link (the endpoint is mounted by the same flag).
	Register struct {
		Email bool `json:"email"`
	} `json:"register"`
	APIBase string `json:"apiBase"`
}

// NewPublicConfig builds the panel config from the app config.
func NewPublicConfig(cfg *config.AppConfig) PublicConfig {
	var pc PublicConfig
	pc.Login.Google = cfg.Admin.LoginGoogle
	pc.Login.Email = cfg.Admin.LoginEmail
	pc.Register.Email = cfg.Admin.RegisterEmail && cfg.Admin.LoginEmail
	pc.Firebase.APIKey = cfg.Admin.WebAPIKey
	pc.Firebase.AuthDomain = cfg.Admin.WebAuthDomain
	pc.Firebase.AppID = cfg.Admin.WebAppID
	if cfg.Firebase != nil {
		pc.Firebase.ProjectID = cfg.Firebase.ProjectID
	}
	pc.APIBase = "/api/" + cfg.App.Version
	return pc
}

// Register mounts /admin/config.json and the SPA under /admin. Call it only when the panel is enabled.
func Register(app *fiber.App, cfg *config.AppConfig) {
	register(app, cfg, distFS)
}

func register(app *fiber.App, cfg *config.AppConfig, files fs.FS) {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		sub = files
	}
	pc := NewPublicConfig(cfg)

	app.Get(basePath+"/config.json", func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		return c.JSON(pc)
	})
	app.Get(basePath, func(c fiber.Ctx) error { return serve(c, sub) })
	app.Get(basePath+"/*", func(c fiber.Ctx) error { return serve(c, sub) })
}

// serve returns the requested asset, or index.html for any path without a file (client-side routes).
func serve(c fiber.Ctx, sub fs.FS) error {
	name := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(c.Path(), basePath)), "/")
	if name == "" {
		name = "index.html"
	}
	data, err := fs.ReadFile(sub, name)
	if err != nil {
		// Missing asset files stay 404; unknown routes fall back to the app shell.
		if path.Ext(name) != "" && path.Ext(name) != ".html" {
			return c.SendStatus(fiber.StatusNotFound)
		}
		name = "index.html"
		data, err = fs.ReadFile(sub, name)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("admin panel is not built into this image")
		}
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		c.Set("Content-Type", ct)
	}
	if name == "index.html" {
		c.Set("Cache-Control", "no-store")
	} else if strings.HasPrefix(name, "assets/") {
		c.Set("Cache-Control", "public, max-age=31536000, immutable") // Vite hashes these file names
	}
	return c.Send(data)
}
