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

[infra.databases.auth]
connection_string = "postgres://user:pass@localhost:5432/auth"

[infra.caches.redis]
url = "redis://localhost:6379"

[[infra.pubsub]]
provider = "nsq"

[infra.pubsub.nsq]
hosts = "localhost:4150"

[infra.pubsub.topics.events]
name = "events-topic"

[infra.pubsub.topics.events.subscriptions.emailer]
name = "emailer-sub"

[[infra.objects]]
provider = "s3"

[infra.objects.s3]
endpoint = "https://s3.amazonaws.com"
region = "us-east-1"
access_key_id = "env:AWS_ACCESS_KEY_ID"
secret_access_key = "env:AWS_SECRET_ACCESS_KEY"

[infra.objects.buckets.uploads]
name = "my-uploads-bucket"
public_base_url = "https://cdn.example.com/uploads"
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

	if f.Infra == nil {
		t.Fatalf("expected Infra to be non-nil")
	}

	if f.Infra.Databases["auth"].ConnectionString != "postgres://user:pass@localhost:5432/auth" {
		t.Errorf("expected infra.databases.auth.connection_string, got %q", f.Infra.Databases["auth"].ConnectionString)
	}

	if f.Infra.Caches["redis"].URL != "redis://localhost:6379" {
		t.Errorf("expected infra.caches.redis.url, got %q", f.Infra.Caches["redis"].URL)
	}

	if len(f.Infra.PubSub) != 1 {
		t.Fatalf("expected 1 pubsub provider, got %d", len(f.Infra.PubSub))
	}
	if f.Infra.PubSub[0].Provider != "nsq" {
		t.Errorf("expected pubsub provider nsq, got %q", f.Infra.PubSub[0].Provider)
	}
	if f.Infra.PubSub[0].NSQ == nil || f.Infra.PubSub[0].NSQ.Hosts != "localhost:4150" {
		t.Errorf("expected pubsub.nsq.hosts localhost:4150, got %v", f.Infra.PubSub[0].NSQ)
	}
	if f.Infra.PubSub[0].Topics["events"].Name != "events-topic" {
		t.Errorf("expected pubsub.topics.events.name events-topic, got %q", f.Infra.PubSub[0].Topics["events"].Name)
	}
	if f.Infra.PubSub[0].Topics["events"].Subscriptions["emailer"].Name != "emailer-sub" {
		t.Errorf("expected pubsub.topics.events.subscriptions.emailer.name emailer-sub, got %q", f.Infra.PubSub[0].Topics["events"].Subscriptions["emailer"].Name)
	}

	if len(f.Infra.Objects) != 1 {
		t.Fatalf("expected 1 object provider, got %d", len(f.Infra.Objects))
	}
	if f.Infra.Objects[0].Provider != "s3" {
		t.Errorf("expected objects provider s3, got %q", f.Infra.Objects[0].Provider)
	}
	if f.Infra.Objects[0].S3 == nil || f.Infra.Objects[0].S3.Region != "us-east-1" {
		t.Errorf("expected objects.s3.region us-east-1, got %v", f.Infra.Objects[0].S3)
	}
	if f.Infra.Objects[0].Buckets["uploads"].Name != "my-uploads-bucket" {
		t.Errorf("expected objects.buckets.uploads.name my-uploads-bucket, got %q", f.Infra.Objects[0].Buckets["uploads"].Name)
	}
}

func TestValueResolve(t *testing.T) {
	tests := []struct {
		name     string
		value    Value
		getEnv   func(string) string
		expected string
		ok       bool
	}{
		{
			name:     "literal value",
			value:    "hello world",
			getEnv:   func(string) string { return "" },
			expected: "hello world",
			ok:       true,
		},
		{
			name:  "env variable exists",
			value: "env:MY_VAR",
			getEnv: func(s string) string {
				if s == "MY_VAR" {
					return "my_value"
				}
				return ""
			},
			expected: "my_value",
			ok:       true,
		},
		{
			name:     "env variable missing",
			value:    "env:MISSING_VAR",
			getEnv:   func(string) string { return "" },
			expected: "",
			ok:       false,
		},
		{
			name:     "empty value",
			value:    "",
			getEnv:   func(string) string { return "" },
			expected: "",
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.value.Resolve(tt.getEnv)
			if ok != tt.ok {
				t.Errorf("expected ok=%v, got ok=%v", tt.ok, ok)
			}
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
