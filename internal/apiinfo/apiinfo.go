// Package apiinfo is the admin panel's "what is running" answer: the API version, the build, the Cloud Run revision,
// the catalog versions and the client version limits. It sits behind the admin role (GET /api/<v>/admin/info); the
// public GET /version keeps only the commit and the client versions.
package apiinfo

import (
	"os"
	"runtime"
	"time"

	"api-library/internal/buildinfo"
	"api-library/internal/config"

	"github.com/gofiber/fiber/v3"
)

// Catalog is one embedded table the service validated at boot.
type Catalog struct {
	Version int `json:"version"`
	Entries int `json:"entries"`
}

// Response is the JSON of GET /admin/info. Nothing secret: no keys, no URLs of other services.
type Response struct {
	APIVersion string `json:"apiVersion"` // app.version of the config, the /api/<v> segment
	APIBase    string `json:"apiBase"`
	Env        string `json:"env"`
	AuthMode   string `json:"authMode"`
	Commit     string `json:"commit"`
	GoVersion  string `json:"goVersion"`

	StartedAt     time.Time `json:"startedAt"`
	ServerTime    time.Time `json:"serverTime"`
	UptimeSeconds int64     `json:"uptimeSeconds"`

	// Cloud Run sets these on a running revision; empty on a local run.
	CloudRun struct {
		Service       string `json:"service"`
		Revision      string `json:"revision"`
		Configuration string `json:"configuration"`
	} `json:"cloudRun"`

	Client struct {
		MinVersion    string `json:"minVersion"`
		LatestVersion string `json:"latestVersion"`
	} `json:"client"`

	Admin struct {
		PanelEnabled      bool `json:"panelEnabled"`
		LoginGoogle       bool `json:"loginGoogle"`
		LoginEmail        bool `json:"loginEmail"`
		RegisterEmail     bool `json:"registerEmail"`
		AcceptCustomToken bool `json:"acceptCustomToken"`
	} `json:"admin"`

	Catalogs map[string]Catalog `json:"catalogs"`
}

// Handler serves GET /admin/info.
type Handler struct {
	cfg     *config.AppConfig
	catalog map[string]Catalog
	started time.Time
	now     func() time.Time
}

// NewHandler captures the config and the catalog versions known at boot. The process start time is taken now.
func NewHandler(cfg *config.AppConfig, catalogs map[string]Catalog) *Handler {
	return &Handler{cfg: cfg, catalog: catalogs, started: time.Now().UTC(), now: func() time.Time { return time.Now().UTC() }}
}

// Info builds the answer.
func (h *Handler) Info() Response {
	now := h.now()
	r := Response{
		APIVersion:    h.cfg.App.Version,
		APIBase:       "/api/" + h.cfg.App.Version,
		Env:           h.cfg.App.Env,
		AuthMode:      h.cfg.App.AuthMode,
		Commit:        buildinfo.Commit,
		GoVersion:     runtime.Version(),
		StartedAt:     h.started,
		ServerTime:    now,
		UptimeSeconds: int64(now.Sub(h.started).Seconds()),
		Catalogs:      h.catalog,
	}
	r.CloudRun.Service = os.Getenv("K_SERVICE")
	r.CloudRun.Revision = os.Getenv("K_REVISION")
	r.CloudRun.Configuration = os.Getenv("K_CONFIGURATION")
	r.Client.MinVersion = h.cfg.Client.MinVersion
	r.Client.LatestVersion = h.cfg.Client.LatestVersion
	r.Admin.PanelEnabled = h.cfg.Admin.PanelEnabled
	r.Admin.LoginGoogle = h.cfg.Admin.LoginGoogle
	r.Admin.LoginEmail = h.cfg.Admin.LoginEmail
	r.Admin.RegisterEmail = h.cfg.Admin.RegisterEmail
	r.Admin.AcceptCustomToken = h.cfg.Admin.AcceptCustomToken
	if r.Catalogs == nil {
		r.Catalogs = map[string]Catalog{}
	}
	return r
}

// Get is the route handler. The route must already demand the admin role.
func (h *Handler) Get(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	return c.JSON(h.Info())
}
