package tomlconfig

import (
	"testing"

	"encore.dev/appruntime/exported/experiments"
)

func TestParse(t *testing.T) {
	data := []byte(`
id = "my-app"
language = "typescript"
experiments = ["exp1"]

[log]
level = "debug"

[cors]
allow_headers = ["*"]
expose_headers = ["X-Special"]

[build]
cgo_enabled = true

[build.docker]
base_image = "alpine"

[migrations]
strategy = "atlas"
`)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.ID != "my-app" {
		t.Errorf("expected ID my-app, got %q", f.ID)
	}
	if f.Language != "typescript" {
		t.Errorf("expected Language typescript, got %q", f.Language)
	}
	if f.Log.Level != "debug" {
		t.Errorf("expected Log.Level debug, got %q", f.Log.Level)
	}
	if len(f.Experiments) != 1 || f.Experiments[0] != experiments.Name("exp1") {
		t.Errorf("expected experiments [exp1], got %v", f.Experiments)
	}
	if f.CORS == nil || len(f.CORS.AllowHeaders) != 1 || f.CORS.AllowHeaders[0] != "*" {
		t.Errorf("expected CORS.AllowHeaders [*], got %v", f.CORS)
	}
	if !f.Build.CgoEnabled {
		t.Errorf("expected Build.CgoEnabled true")
	}
	if f.Build.Docker.BaseImage != "alpine" {
		t.Errorf("expected build.docker.base_image alpine, got %q", f.Build.Docker.BaseImage)
	}
	if f.Migrations.Strategy != "atlas" {
		t.Errorf("expected migrations.strategy atlas, got %q", f.Migrations.Strategy)
	}
}

func TestParse_DefaultLanguage(t *testing.T) {
	data := []byte(`id = "my-app"`)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Language != "go" {
		t.Errorf("expected default Language go, got %q", f.Language)
	}
}
