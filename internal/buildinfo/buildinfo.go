// Package buildinfo holds what the build stamps into the binary. The Cloud Run image build sets Commit with
// -ldflags "-X api-library/internal/buildinfo.Commit=<short git hash>" (cloudbuild.yaml passes it from deploy.bat),
// so a running revision can say which commit it was built from (GET /healthz, the [BOOT] log line).
package buildinfo

// Commit is the short git hash of the build, with a "-dirty" suffix when the working tree had uncommitted changes.
// "dev" for a plain `go build` / `go run`.
var Commit = "dev"
