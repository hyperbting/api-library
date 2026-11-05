package apiinfo

import (
	"testing"
	"time"

	"api-library/internal/config"
)

func TestInfo(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.App.Version = "v1"
	cfg.App.Env = "production"
	cfg.Client.MinVersion = "1.0"
	cfg.Admin.PanelEnabled = true
	t.Setenv("K_SERVICE", "api-library-admin")
	t.Setenv("K_REVISION", "api-library-admin-00007-abc")

	h := NewHandler(cfg, map[string]Catalog{"quests": {Version: 26, Entries: 12, Steps: 63}})
	h.now = func() time.Time { return h.started.Add(90 * time.Second) }
	r := h.Info()

	if r.APIVersion != "v1" || r.APIBase != "/api/v1" || r.Env != "production" {
		t.Errorf("api: %+v", r)
	}
	if r.CloudRun.Service != "api-library-admin" || r.CloudRun.Revision != "api-library-admin-00007-abc" {
		t.Errorf("cloud run: %+v", r.CloudRun)
	}
	if r.UptimeSeconds != 90 {
		t.Errorf("uptime = %d, want 90", r.UptimeSeconds)
	}
	if r.Client.MinVersion != "1.0" || !r.Admin.PanelEnabled {
		t.Errorf("client/admin: %+v %+v", r.Client, r.Admin)
	}
	if c := r.Catalogs["quests"]; c.Version != 26 || c.Entries != 12 || c.Steps != 63 {
		t.Errorf("catalogs: %+v", r.Catalogs)
	}
	if r.GoVersion == "" || r.Commit == "" {
		t.Errorf("build: %+v", r)
	}
	// A handler built without catalogs still answers an object, not null.
	if NewHandler(cfg, nil).Info().Catalogs == nil {
		t.Error("catalogs is nil")
	}
}

func TestCatalogsHandlerReturnsTheLoadedFiles(t *testing.T) {
	h := NewCatalogsHandler(map[string]CatalogFile{"items": {Path: "internal/item/data/items.json", Version: 2, Data: map[string]int{"n": 7}}})
	r := h.Catalogs()
	if r.Repo != RepoURL || r.Catalogs["items"].Path != "internal/item/data/items.json" || r.Catalogs["items"].Version != 2 {
		t.Fatalf("unexpected answer: %+v", r)
	}
	if got := NewCatalogsHandler(nil).Catalogs(); got.Catalogs == nil {
		t.Fatal("a nil map must become an empty one so the page gets {} not null")
	}
}
