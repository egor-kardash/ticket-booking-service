package config_test

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/config"
)

const testJWTSecret = "0123456789abcdef0123456789abcdef"

func validConfig() config.Config {
	return config.Config{
		HTTPServer: config.HTTPServer{
			Address:         "localhost:8080",
			Timeout:         5 * time.Second,
			IdleTimeout:     60 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Database: config.Database{
			Host: "localhost", Port: 5433, User: "booking", Password: "secret", Name: "booking", SSLMode: "disable",
		},
		Log:  config.Log{Level: "info", Format: "text"},
		Auth: config.Auth{BcryptCost: 12, JWTSecret: testJWTSecret, TokenDuration: 15 * time.Minute},
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr string
	}{
		{name: "accepts valid config"},

		// Обязательные поля: каждое даёт собственную ошибку.
		{name: "requires database user", mutate: func(c *config.Config) { c.Database.User = "" }, wantErr: "database.user"},
		{name: "requires database password", mutate: func(c *config.Config) { c.Database.Password = "" }, wantErr: "database.password"},
		{name: "requires database name", mutate: func(c *config.Config) { c.Database.Name = "" }, wantErr: "database.name"},
		{name: "requires jwt secret", mutate: func(c *config.Config) { c.Auth.JWTSecret = "" }, wantErr: "jwt_secret"},

		// Границы: байты, а не символы.
		{name: "rejects jwt secret of 31 bytes", mutate: func(c *config.Config) { c.Auth.JWTSecret = strings.Repeat("s", 31) }, wantErr: "at least 32 bytes"},
		{name: "accepts jwt secret of 32 bytes", mutate: func(c *config.Config) { c.Auth.JWTSecret = strings.Repeat("s", 32) }},

		{name: "rejects zero token duration", mutate: func(c *config.Config) { c.Auth.TokenDuration = 0 }, wantErr: "token_duration"},
		{name: "accepts one second token duration", mutate: func(c *config.Config) { c.Auth.TokenDuration = time.Second }},

		{name: "rejects bcrypt cost below range", mutate: func(c *config.Config) { c.Auth.BcryptCost = 9 }, wantErr: "bcrypt_cost"},
		{name: "accepts bcrypt cost lower bound", mutate: func(c *config.Config) { c.Auth.BcryptCost = 10 }},
		{name: "accepts bcrypt cost upper bound", mutate: func(c *config.Config) { c.Auth.BcryptCost = 14 }},
		{name: "rejects bcrypt cost above range", mutate: func(c *config.Config) { c.Auth.BcryptCost = 15 }, wantErr: "bcrypt_cost"},

		// Один representative-кейс на диапазон портов.
		{name: "rejects http port above 65535", mutate: func(c *config.Config) { c.HTTPServer.Address = "localhost:65536" }, wantErr: "http_server.port"},

		{name: "rejects unknown sslmode", mutate: func(c *config.Config) { c.Database.SSLMode = "sometimes" }, wantErr: "sslmode"},
		{name: "accepts verify-full sslmode", mutate: func(c *config.Config) { c.Database.SSLMode = "verify-full" }},

		{name: "rejects tls cert without key", mutate: func(c *config.Config) { c.TLS.CertPath = "/certs/cert.pem" }, wantErr: "set together"},
		{name: "accepts tls pair", mutate: func(c *config.Config) {
			c.TLS.CertPath = "/certs/cert.pem"
			c.TLS.KeyPath = "/certs/key.pem"
		}},

		{name: "rejects unknown log format", mutate: func(c *config.Config) { c.Log.Format = "xml" }, wantErr: "log.format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			err := cfg.Validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestDatabaseDSN(t *testing.T) {
	db := config.Database{
		Host: "db.internal", Port: 6543, User: "booking", Password: "p@ss w/rd%", Name: "booking", SSLMode: "require",
	}

	u, err := url.Parse(db.DSN())
	if err != nil {
		t.Fatalf("DSN is not a valid URL: %v (dsn %q)", err, db.DSN())
	}

	if u.Scheme != "postgres" {
		t.Errorf("scheme = %q, want postgres", u.Scheme)
	}
	if got := u.User.Username(); got != "booking" {
		t.Errorf("user = %q, want booking", got)
	}
	password, ok := u.User.Password()
	if !ok || password != "p@ss w/rd%" {
		t.Errorf("password = %q, want %q: special characters must survive encoding", password, "p@ss w/rd%")
	}
	// Если бы @ или / не кодировались, host и path разъехались бы.
	if u.Host != "db.internal:6543" {
		t.Errorf("host = %q, want db.internal:6543", u.Host)
	}
	if u.Path != "/booking" {
		t.Errorf("path = %q, want /booking", u.Path)
	}
	if got := u.Query().Get("sslmode"); got != "require" {
		t.Errorf("sslmode = %q, want require", got)
	}
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	return path
}

// unsetEnv отличается от t.Setenv(key, ""): пустая строка перекрыла бы env-default,
// а для проверки дефолтов переменная должна отсутствовать.
func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()

	for _, key := range keys {
		value, existed := os.LookupEnv(key)
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(key, value)
				return
			}
			_ = os.Unsetenv(key)
		})

		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
}

// TestLoad проверяет три вещи сразу: env перебивает YAML, значения без
// env-тега берутся из YAML, а незаданные поля получают env-default.
func TestLoad(t *testing.T) {
	unsetEnv(t,
		"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_SSLMODE",
		"LOG_LEVEL", "LOG_FORMAT", "TLS_CERT_PATH", "TLS_KEY_PATH",
		"BCRYPT_COST", "TOKEN_DURATION",
	)
	t.Setenv("HTTP_ADDR", "127.0.0.1:9090")  // env перебивает YAML
	t.Setenv("POSTGRES_HOST", "db.internal") // env перебивает YAML
	t.Setenv("POSTGRES_USER", "booking")
	t.Setenv("POSTGRES_PASSWORD", "s3cret")
	t.Setenv("POSTGRES_DB", "booking")
	t.Setenv("JWT_SECRET", testJWTSecret)

	cfg, err := config.Load(writeConfigFile(t, `
http_server:
  address: "localhost:8080"
  timeout: 5s
database:
  host: yaml-host
`))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	want := config.Config{
		HTTPServer: config.HTTPServer{Address: "127.0.0.1:9090", Timeout: 5 * time.Second, IdleTimeout: 60 * time.Second, ShutdownTimeout: 10 * time.Second},
		Database:   config.Database{Host: "db.internal", Port: 5433, User: "booking", Password: "s3cret", Name: "booking", SSLMode: "disable"},
		Log:        config.Log{Level: "info", Format: "text"},
		Auth:       config.Auth{BcryptCost: 12, JWTSecret: testJWTSecret, TokenDuration: 15 * time.Minute},
	}

	if *cfg != want {
		t.Errorf("Load() = %+v, want %+v", *cfg, want)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Run("wraps missing file", func(t *testing.T) {
		_, err := config.Load(filepath.Join(t.TempDir(), "absent.yml"))
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Load() error = %v, want errors.Is(fs.ErrNotExist): lost %%w", err)
		}
	})

	t.Run("rejects malformed yaml", func(t *testing.T) {
		_, err := config.Load(writeConfigFile(t, "http_server: {address: localhost:8080"))
		if err == nil {
			t.Fatal("Load() = nil, want error")
		}
		if !strings.Contains(err.Error(), "read config") {
			t.Errorf("Load() error = %q, want wrapping %q", err, "read config")
		}
	})
}
