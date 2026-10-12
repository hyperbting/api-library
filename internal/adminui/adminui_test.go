package adminui

import (
	"api-library/internal/config"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v3"
)

func testApp(t *testing.T, cfg *config.AppConfig) *fiber.App {
	t.Helper()
	files := fstest.MapFS{
		"dist/index.html":        {Data: []byte("<html>panel</html>")},
		"dist/assets/app-abc.js": {Data: []byte("console.log(1)")},
	}
	app := fiber.New()
	register(app, cfg, files)
	return app
}

func get(t *testing.T, app *fiber.App, path string) (int, string, string) {
	t.Helper()
	res, err := app.Test(httptest.NewRequest("GET", path, nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body), res.Header.Get("Content-Type")
}

func TestServe(t *testing.T) {
	app := testApp(t, &config.AppConfig{})

	if code, body, _ := get(t, app, "/admin/"); code != 200 || body != "<html>panel</html>" {
		t.Fatalf("index: %d %q", code, body)
	}
	if code, body, _ := get(t, app, "/admin"); code != 200 || body != "<html>panel</html>" {
		t.Fatalf("index without slash: %d %q", code, body)
	}
	if code, body, _ := get(t, app, "/admin/login"); code != 200 || body != "<html>panel</html>" {
		t.Fatalf("spa fallback: %d %q", code, body)
	}
	if code, _, ct := get(t, app, "/admin/assets/app-abc.js"); code != 200 || ct == "" {
		t.Fatalf("asset: %d %q", code, ct)
	}
	if code, _, _ := get(t, app, "/admin/assets/missing.js"); code != 404 {
		t.Fatalf("missing asset: %d", code)
	}
	if code, body, _ := get(t, app, "/admin/../../etc/passwd"); code == 200 && body != "<html>panel</html>" {
		t.Fatalf("path traversal served %q", body)
	}
}

func TestConfigJSON(t *testing.T) {
	cfg := &config.AppConfig{
		App:      config.AppDetail{Version: "v99"},
		Firebase: &config.FirebaseConfig{ProjectID: "proj"},
		Admin:    config.AdminConfig{LoginEmail: true, LoginGoogle: false, WebAPIKey: "k"},
	}
	app := testApp(t, cfg)

	code, body, _ := get(t, app, "/admin/config.json")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	var pc PublicConfig
	if err := json.Unmarshal([]byte(body), &pc); err != nil {
		t.Fatal(err)
	}
	if !pc.Login.Email || pc.Login.Google {
		t.Fatalf("login flags: %+v", pc.Login)
	}
	if pc.Firebase.ProjectID != "proj" || pc.Firebase.APIKey != "k" || pc.APIBase != "/api/v99" {
		t.Fatalf("config: %+v", pc)
	}
}

func TestDistEmbedCompiles(t *testing.T) {
	// The embedded dist must exist (only .gitkeep outside the Docker build) so `go build` never fails.
	if _, err := distFS.ReadDir("dist"); err != nil {
		t.Fatal(err)
	}
}
