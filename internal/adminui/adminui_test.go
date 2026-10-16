package adminui

import (
	"api-library/internal/config"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v3"
)

func testApp(t *testing.T, cfg *config.AppConfig) *fiber.App {
	t.Helper()
	files := fstest.MapFS{
		"dist/index.html":        {Data: []byte("<html>panel</html>")},
		"dist/assets/app-abc.js": {Data: []byte("console.log(1)")},
		"dist/assets/big-abc.js": {Data: []byte(strings.Repeat("function f(){return 1}\n", 200))},
		"dist/assets/font.woff2": {Data: []byte(strings.Repeat("w", 2000))},
		"dist/favicon.ico":       {Data: []byte{0, 0, 1, 0, 1, 0}},
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
	if code, _, ct := get(t, app, "/admin/favicon.ico"); code != 200 || ct != "image/x-icon" {
		t.Fatalf("favicon: %d %q", code, ct)
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
	if pc.Register.Email {
		t.Fatalf("register link must be off by default: %+v", pc.Register)
	}
}

// The link shows only with ADMIN_REGISTER_EMAIL and email sign-in: the account can only sign in with email.
func TestConfigJSONRegisterFlag(t *testing.T) {
	for _, tc := range []struct {
		name  string
		admin config.AdminConfig
		want  bool
	}{
		{"flag and email sign-in", config.AdminConfig{LoginEmail: true, RegisterEmail: true}, true},
		{"flag without email sign-in", config.AdminConfig{RegisterEmail: true}, false},
		{"no flag", config.AdminConfig{LoginEmail: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := testApp(t, &config.AppConfig{Admin: tc.admin})
			_, body, _ := get(t, app, "/admin/config.json")
			var pc PublicConfig
			if err := json.Unmarshal([]byte(body), &pc); err != nil {
				t.Fatal(err)
			}
			if pc.Register.Email != tc.want {
				t.Fatalf("register.email = %v want %v", pc.Register.Email, tc.want)
			}
		})
	}
}

func TestDistEmbedCompiles(t *testing.T) {
	// The embedded dist must exist (only .gitkeep outside the Docker build) so `go build` never fails.
	if _, err := distFS.ReadDir("dist"); err != nil {
		t.Fatal(err)
	}
}

func getEncoded(t *testing.T, app *fiber.App, path, accept string) (*http.Response, []byte) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if accept != "" {
		req.Header.Set("Accept-Encoding", accept)
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	return res, body
}

func TestGzipAssets(t *testing.T) {
	app := testApp(t, &config.AppConfig{})
	plain := strings.Repeat("function f(){return 1}\n", 200)

	// A client that accepts gzip gets a smaller, correct body.
	res, body := getEncoded(t, app, "/admin/assets/big-abc.js", "gzip, br")
	if res.Header.Get("Content-Encoding") != "gzip" || res.Header.Get("Vary") != "Accept-Encoding" {
		t.Fatalf("headers: %v", res.Header)
	}
	if len(body) >= len(plain) {
		t.Fatalf("gzip body %d bytes, plain %d", len(body), len(plain))
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != plain {
		t.Fatal("decompressed body differs")
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("content type %q", ct)
	}

	// The second request is served from the cache with the same bytes.
	_, again := getEncoded(t, app, "/admin/assets/big-abc.js", "gzip")
	if !bytes.Equal(again, body) {
		t.Fatal("cached gzip differs")
	}

	// No Accept-Encoding: plain bytes, still marked Vary.
	res, body = getEncoded(t, app, "/admin/assets/big-abc.js", "")
	if res.Header.Get("Content-Encoding") != "" || string(body) != plain || res.Header.Get("Vary") != "Accept-Encoding" {
		t.Fatalf("plain: %v len %d", res.Header, len(body))
	}

	// Tiny files and already-compressed fonts are left alone.
	for _, p := range []string{"/admin/assets/app-abc.js", "/admin/assets/font.woff2"} {
		res, _ = getEncoded(t, app, p, "gzip")
		if res.Header.Get("Content-Encoding") != "" {
			t.Fatalf("%s was compressed", p)
		}
	}

	// The index is compressible too, and keeps no-store.
	res, _ = getEncoded(t, app, "/admin/", "gzip")
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("index cache header %q", res.Header.Get("Cache-Control"))
	}
}
