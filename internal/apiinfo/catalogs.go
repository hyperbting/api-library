package apiinfo

import (
	"github.com/gofiber/fiber/v3"

	"api-library/internal/buildinfo"
)

// RepoURL is where the embedded files live; the panel builds a link to the exact commit the service was built from.
const RepoURL = "https://github.com/hyperbting/api-library"

// CatalogFile is one embedded table as the panel's Catalogs page shows it: its path in the repo, its version and the
// parsed content (read only; these files are generated or edited in git, never through the panel).
type CatalogFile struct {
	Path    string `json:"path"`
	Version int    `json:"version"`
	Data    any    `json:"data"`
}

// CatalogsResponse is the JSON of GET /admin/catalogs. Commit is the build's short git hash ("dev" on a local run,
// possibly with a "-dirty" suffix): the panel links the file at that commit only when it is a real hash.
type CatalogsResponse struct {
	Commit   string                 `json:"commit"`
	Repo     string                 `json:"repo"`
	Catalogs map[string]CatalogFile `json:"catalogs"`
}

// CatalogsHandler serves GET /admin/catalogs.
type CatalogsHandler struct {
	files map[string]CatalogFile
}

// NewCatalogsHandler captures the catalogs loaded at boot, so the page shows what the running revision serves.
func NewCatalogsHandler(files map[string]CatalogFile) *CatalogsHandler {
	return &CatalogsHandler{files: files}
}

// Catalogs builds the answer.
func (h *CatalogsHandler) Catalogs() CatalogsResponse {
	files := h.files
	if files == nil {
		files = map[string]CatalogFile{}
	}
	return CatalogsResponse{Commit: buildinfo.Commit, Repo: RepoURL, Catalogs: files}
}

// Get is the route handler. The route must already demand the admin role.
func (h *CatalogsHandler) Get(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	return c.JSON(h.Catalogs())
}
