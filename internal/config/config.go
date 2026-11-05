package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	HTTPServer HTTPServer `yaml:"http_server"`
	Database   DBConfig   `yaml:"database"`
	Log        LogConfig  `yaml:"log"`
}

type HTTPServer struct {
	Address         string        `yaml:"address" env:"HTTP_ADDR" env-default:"localhost:8080"`
	Timeout         time.Duration `yaml:"timeout" env-default:"5s"`
	IdleTimeout     time.Duration `yaml:"idle_timeout" env-default:"60s"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env-default:"10s"`
}

type LogConfig struct {
	Level  string `yaml:"level"  env:"LOG_LEVEL"  env-default:"info"`
	Format string `yaml:"format" env:"LOG_FORMAT" env-default:"text"`
}

type DBConfig struct {
	Host     string `env:"POSTGRES_HOST" env-default:"localhost"`
	Port     int    `env:"POSTGRES_PORT" env-default:"5433"`
	User     string `env:"POSTGRES_USER" env-required:"true"`
	Password string `env:"POSTGRES_PASSWORD" env-required:"true"`
	Name     string `env:"POSTGRES_DB" env-required:"true"`
	SSLMode  string `env:"POSTGRES_SSLMODE" env-default:"disable"`
	Schema   string `env:"POSTGRES_SCHEMA" env-default:"booking"`
}

func (db DBConfig) DSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(db.User, db.Password),
		Host:   net.JoinHostPort(db.Host, strconv.Itoa(db.Port)),
		Path:   "/" + db.Name,
	}

	q := u.Query()
	q.Set("sslmode", db.SSLMode)
	if db.Schema != "" {
		q.Set("search_path", db.Schema)
	}
	u.RawQuery = q.Encode()

	return u.String()
}

func (c *Config) Validate() error {
	_, port, err := net.SplitHostPort(c.HTTPServer.Address)
	if err != nil {
		return fmt.Errorf("invalid HTTP address %q: %w", c.HTTPServer.Address, err)
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid HTTP port %q", port)
	}
	if portNum < 1 || portNum > 65535 {
		return errors.New("http port out of range (1-65535)")
	}

	if c.HTTPServer.Timeout <= 0 {
		return errors.New("timeout must be greater than 0")
	}
	if c.HTTPServer.IdleTimeout <= 0 {
		return errors.New("idle timeout must be greater than 0")
	}
	if c.HTTPServer.ShutdownTimeout <= 0 {
		return errors.New("shutdown timeout must be greater than 0")
	}

	if c.Database.Host == "" {
		return errors.New("db host is required")
	}
	if c.Database.Name == "" {
		return errors.New("db name is required")
	}
	if c.Database.Password == "" {
		return errors.New("db password is required")
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		return errors.New("db port out of range (1-65535)")
	}
	if c.Database.User == "" {
		return errors.New("db user is required")
	}
	if c.Database.Schema == "" {
		return errors.New("db schema is required")
	}

	switch c.Database.SSLMode {
	case "disable", "require", "verify-ca", "verify-full", "allow", "prefer":
	default:
		return fmt.Errorf("unsupported db sslmode %q", c.Database.SSLMode)
	}

	return nil
}

func Load(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("config path is empty")
	}

	var cfg Config
	if err := cleanenv.ReadConfig(path, &cfg); err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}
