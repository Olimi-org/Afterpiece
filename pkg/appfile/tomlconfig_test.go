package appfile

import (
	"testing"

	"encore.dev/appruntime/exported/experiments"
)

func TestParse(t *testing.T) {
	data := []byte(`
id = "my-app"
experiments = ["exp1"]

log_level = "debug"

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
	f, err := Parse(data, "toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.ID != "my-app" {
		t.Errorf("expected ID my-app, got %q", f.ID)
	}

	if f.LogLevel != "debug" {
		t.Errorf("expected Log.Level debug, got %q", f.LogLevel)
	}

	if len(f.Experiments) != 1 || f.Experiments[0] != experiments.Name("exp1") {
		t.Errorf("expected experiments [exp1], got %v", f.Experiments)
	}

	if f.GlobalCORS == nil || len(f.GlobalCORS.AllowHeaders) != 1 || f.GlobalCORS.AllowHeaders[0] != "*" {
		t.Errorf("expected GlobalCORS.AllowHeaders [*], got %v", f.GlobalCORS)
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
