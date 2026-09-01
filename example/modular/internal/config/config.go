package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

// Module is a leaf module: it owns the validated runtime configuration. It has
// no app surface. Other modules wire against it and read Value.
type Module struct {
	cfg Config
}

type Config struct {
	HTTPAddr    string
	SourceRoot  string
	Concurrency int

	Postgres Postgres
	Redis    Redis
	Storage  Storage
	SMTP     SMTP
	I18n     I18n
}

type Postgres struct {
	URL string
}

type Redis struct {
	Addr     string
	Password string
	DB       int
}

type Storage struct {
	Bucket string
	Region string
}

type SMTP struct {
	Host string
	Port int
}

type I18n struct {
	Default string
}

func New() *Module { return &Module{} }

// Wire loads configuration from the environment and validates it. A missing or
// malformed value panics so a bad deployment fails loudly at startup.
func (m *Module) Wire() {
	m.cfg = fromEnv()
}

func (m *Module) Value() Config { return m.cfg }

func fromEnv() Config {
	var c Config
	var err error

	with := func(name string, set func(string) error) {
		if err != nil {
			return
		}
		value := os.Getenv(name)
		if value == "" {
			err = fmt.Errorf("%s is required", name)
			return
		}
		if setErr := set(value); setErr != nil {
			err = fmt.Errorf("%s: %w", name, setErr)
		}
	}

	with("HTTP_ADDR", func(v string) error { c.HTTPAddr = v; return nil })
	with("SOURCE_ROOT", func(v string) error { c.SourceRoot = v; return nil })
	with("POSTGRES_URL", func(v string) error { c.Postgres.URL = v; return nil })
	with("REDIS_ADDR", func(v string) error { c.Redis.Addr = v; return nil })
	with("STORAGE_BUCKET", func(v string) error { c.Storage.Bucket = v; return nil })
	with("STORAGE_REGION", func(v string) error { c.Storage.Region = v; return nil })
	with("SMTP_HOST", func(v string) error { c.SMTP.Host = v; return nil })
	with("SMTP_PORT", func(v string) error { return parseInt(v, &c.SMTP.Port) })
	with("I18N_DEFAULT", func(v string) error { c.I18n.Default = v; return nil })

	if err != nil {
		panic(fmt.Errorf("config: %w", err))
	}

	c.Redis.Password = os.Getenv("REDIS_PASSWORD")
	if v := os.Getenv("REDIS_DB"); v != "" {
		n, parseErr := strconv.Atoi(v)
		if parseErr != nil {
			panic(fmt.Errorf("config: REDIS_DB: %w", parseErr))
		}
		c.Redis.DB = n
	}

	if v := os.Getenv("CONCURRENCY"); v != "" {
		n, parseErr := strconv.Atoi(v)
		if parseErr != nil {
			panic(fmt.Errorf("config: CONCURRENCY: %w", parseErr))
		}
		c.Concurrency = n
	}
	if c.Concurrency == 0 {
		c.Concurrency = 10
	}

	if _, parseErr := url.Parse(c.Postgres.URL); parseErr != nil {
		panic(fmt.Errorf("config: POSTGRES_URL: %w", parseErr))
	}
	return c
}

func parseInt(value string, out *int) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	*out = n
	return nil
}
