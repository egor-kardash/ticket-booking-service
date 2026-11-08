package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

const (
	minBcryptCost = 10
	maxBcryptCost = 14
)

type Config struct {
	HTTPServer HTTPServer `yaml:"http_server"`
	Database   Database   `yaml:"database"`
	Log        Log        `yaml:"log"`
	TLS        TLS        `yaml:"tls"`
	Auth       Auth       `yaml:"auth"`
}

type HTTPServer struct {
	Address         string        `yaml:"address" env:"HTTP_ADDR" env-default:"localhost:8080"`
	Timeout         time.Duration `yaml:"timeout" env-default:"5s"`
	IdleTimeout     time.Duration `yaml:"idle_timeout" env-default:"60s"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env-default:"10s"`
}

func (h HTTPServer) Validate() error {
	_, port, err := net.SplitHostPort(h.Address)
	if err != nil {
		return fmt.Errorf("http_server.address %q: %w", h.Address, err)
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("http_server.port %q: %w", port, err)
	}
	if portNum < 1 || portNum > 65535 {
		return errors.New("http_server.port: must be in range 1-65535")
	}

	if h.Timeout <= 0 {
		return errors.New("http_server.timeout: must be greater than 0")
	}
	if h.IdleTimeout <= 0 {
		return errors.New("http_server.idle_timeout: must be greater than 0")
	}
	if h.ShutdownTimeout <= 0 {
		return errors.New("http_server.shutdown_timeout: must be greater than 0")
	}

	return nil
}

type Database struct {
	Host     string `yaml:"host"     env:"POSTGRES_HOST" env-default:"localhost"`
	Port     int    `yaml:"port"     env:"POSTGRES_PORT" env-default:"5433"`
	User     string `yaml:"user"     env:"POSTGRES_USER"`
	Password string `yaml:"-"        env:"POSTGRES_PASSWORD"`
	Name     string `yaml:"name"     env:"POSTGRES_DB"`
	SSLMode  string `yaml:"ssl_mode" env:"POSTGRES_SSLMODE" env-default:"disable"`
}

func (db Database) Validate() error {
	if db.Host == "" {
		return errors.New("database.host: required")
	}
	if db.Port < 1 || db.Port > 65535 {
		return errors.New("database.port: must be in range 1-65535")
	}
	if db.User == "" {
		return errors.New("database.user: required")
	}
	if db.Password == "" {
		return errors.New("database.password: required")
	}
	if db.Name == "" {
		return errors.New("database.name: required")
	}

	switch db.SSLMode {
	case "disable", "require", "verify-ca", "verify-full", "allow", "prefer":
	default:
		return fmt.Errorf("database.sslmode: unsupported value %q", db.SSLMode)
	}

	return nil
}

func (db Database) DSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(db.User, db.Password),
		Host:   net.JoinHostPort(db.Host, strconv.Itoa(db.Port)),
		Path:   "/" + db.Name,
	}

	q := u.Query()
	q.Set("sslmode", db.SSLMode)
	u.RawQuery = q.Encode()

	return u.String()
}

type Log struct {
	Level  string `yaml:"level"  env:"LOG_LEVEL"  env-default:"info"`
	Format string `yaml:"format" env:"LOG_FORMAT" env-default:"text"`
}

func (l Log) Validate() error {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(l.Level)); err != nil {
		return fmt.Errorf("log.level %q: %w", l.Level, err)
	}

	switch l.Format {
	case "text", "json":
		return nil
	default:
		return fmt.Errorf("log.format %q: must be one of text, json", l.Format)
	}
}

type TLS struct {
	CertPath string `yaml:"cert_path" env:"TLS_CERT_PATH"`
	KeyPath  string `yaml:"key_path"  env:"TLS_KEY_PATH"`
}

func (t TLS) Validate() error {
	if (t.CertPath == "") != (t.KeyPath == "") {
		return errors.New("tls.cert_path and tls.key_path must be set together")
	}
	return nil
}

func (t TLS) Enabled() bool {
	return t.CertPath != "" && t.KeyPath != ""
}

type Auth struct {
	BcryptCost    int           `yaml:"bcrypt_cost" env:"BCRYPT_COST" env-default:"12"`
	JWTSecret     string        `yaml:"-"          env:"JWT_SECRET"`
	TokenDuration time.Duration `yaml:"token_duration" env:"TOKEN_DURATION" env-default:"15m"`
}

func (a Auth) Validate() error {
	if a.JWTSecret == "" {
		return errors.New("auth.jwt_secret: required")
	}
	if len(a.JWTSecret) < 32 {
		return errors.New("auth.jwt_secret: must be at least 32 bytes")
	}
	if a.TokenDuration < time.Second {
		return errors.New("auth.token_duration: must be at least 1s")
	}
	if a.BcryptCost < minBcryptCost || a.BcryptCost > maxBcryptCost {
		return fmt.Errorf("auth.bcrypt_cost: must be in range %d..%d",
			minBcryptCost, maxBcryptCost)
	}

	return nil
}

func (c *Config) Validate() error {
	if err := c.HTTPServer.Validate(); err != nil {
		return err
	}
	if err := c.Database.Validate(); err != nil {
		return err
	}
	if err := c.Log.Validate(); err != nil {
		return err
	}
	if err := c.TLS.Validate(); err != nil {
		return err
	}
	if err := c.Auth.Validate(); err != nil {
		return err
	}

	return nil
}

func Load(path string) (*Config, error) {
	if path == "" {
		return nil, errors.New("config path is empty")
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
