package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var adminPhonePattern = regexp.MustCompile(`^1\d{10}$`)

type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	value, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", node.Value, err)
	}
	*d = Duration(value)
	return nil
}

func (d Duration) Value() time.Duration { return time.Duration(d) }

// Clock is a wall-clock time of day in the server's local timezone, parsed
// from a "HH:MM" string.
type Clock struct {
	Hour   int
	Minute int
}

func (c *Clock) UnmarshalYAML(node *yaml.Node) error {
	parsed, err := time.Parse("15:04", node.Value)
	if err != nil {
		return fmt.Errorf("invalid clock time %q: want HH:MM", node.Value)
	}
	c.Hour, c.Minute = parsed.Hour(), parsed.Minute()
	return nil
}

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	MySQL    MySQLConfig    `yaml:"mysql"`
	Redis    RedisConfig    `yaml:"redis"`
	Business BusinessConfig `yaml:"business"`
	Security SecurityConfig `yaml:"security"`
}

type ServerConfig struct {
	Host            string   `yaml:"host"`
	Port            int      `yaml:"port"`
	Mode            string   `yaml:"mode"`
	ReadTimeout     Duration `yaml:"read_timeout"`
	WriteTimeout    Duration `yaml:"write_timeout"`
	ShutdownTimeout Duration `yaml:"shutdown_timeout"`
	AllowedOrigins  []string `yaml:"allowed_origins"`
}

type MySQLConfig struct {
	DSN                   string   `yaml:"dsn"`
	MaxIdleConnections    int      `yaml:"max_idle_connections"`
	MaxOpenConnections    int      `yaml:"max_open_connections"`
	ConnectionMaxLifetime Duration `yaml:"connection_max_lifetime"`
}

type RedisConfig struct {
	Address      string   `yaml:"address"`
	Password     string   `yaml:"password"`
	Database     int      `yaml:"database"`
	DialTimeout  Duration `yaml:"dial_timeout"`
	ReadTimeout  Duration `yaml:"read_timeout"`
	WriteTimeout Duration `yaml:"write_timeout"`
}

type BusinessConfig struct {
	AdminPhone               string   `yaml:"admin_phone"`
	QRCodeUploadDir          string   `yaml:"qrcode_upload_dir"`
	QRCodeMaxUploadBytes     int64    `yaml:"qrcode_max_upload_bytes"`
	LuckyCodeMinLength       int      `yaml:"lucky_code_min_length"`
	LuckyCodeMaxLength       int      `yaml:"lucky_code_max_length"`
	ActivityContentMaxLength int      `yaml:"activity_content_max_length"`
	FirstVisitTTL            Duration `yaml:"first_visit_ttl"`
	LuckyClaimTTL            Duration `yaml:"lucky_claim_ttl"`
	DailyResetClock          Clock    `yaml:"daily_reset_time"`
}

type SecurityConfig struct {
	AdminSessionTTL  Duration `yaml:"admin_session_ttl"`
	AdminTokenSecret string   `yaml:"admin_token_secret"`
}

func defaults() Config {
	return Config{
		Server: ServerConfig{
			Host: "0.0.0.0", Port: 8080, Mode: "debug",
			ReadTimeout: Duration(10 * time.Second), WriteTimeout: Duration(15 * time.Second),
			ShutdownTimeout: Duration(10 * time.Second),
			AllowedOrigins:  []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		},
		MySQL: MySQLConfig{
			MaxIdleConnections: 10, MaxOpenConnections: 50,
			ConnectionMaxLifetime: Duration(30 * time.Minute),
		},
		Redis: RedisConfig{
			Address: "127.0.0.1:6379", DialTimeout: Duration(3 * time.Second),
			ReadTimeout: Duration(2 * time.Second), WriteTimeout: Duration(2 * time.Second),
		},
		Business: BusinessConfig{
			QRCodeUploadDir: "uploads", QRCodeMaxUploadBytes: 5 * 1024 * 1024,
			LuckyCodeMinLength: 8, LuckyCodeMaxLength: 9, ActivityContentMaxLength: 200,
			FirstVisitTTL: Duration(365 * 24 * time.Hour), LuckyClaimTTL: Duration(24 * time.Hour),
			DailyResetClock: Clock{Hour: 0, Minute: 0},
		},
		Security: SecurityConfig{AdminSessionTTL: Duration(12 * time.Hour)},
	}
}

func Load(path string) (Config, error) {
	cfg := defaults()
	contents, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(contents, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := applyEnvironment(&cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var problems []string
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		problems = append(problems, "server.port must be between 1 and 65535")
	}
	if c.Server.Mode != "debug" && c.Server.Mode != "release" && c.Server.Mode != "test" {
		problems = append(problems, "server.mode must be debug, release, or test")
	}
	if strings.TrimSpace(c.MySQL.DSN) == "" {
		problems = append(problems, "mysql.dsn is required")
	}
	if strings.TrimSpace(c.Redis.Address) == "" {
		problems = append(problems, "redis.address is required")
	}
	if c.Server.ReadTimeout.Value() <= 0 || c.Server.WriteTimeout.Value() <= 0 || c.Server.ShutdownTimeout.Value() <= 0 {
		problems = append(problems, "server timeouts must be positive")
	}
	if c.MySQL.MaxIdleConnections < 0 || c.MySQL.MaxOpenConnections < 1 {
		problems = append(problems, "mysql connection pool values are invalid")
	}
	if c.Redis.DialTimeout.Value() <= 0 || c.Redis.ReadTimeout.Value() <= 0 || c.Redis.WriteTimeout.Value() <= 0 {
		problems = append(problems, "redis timeouts must be positive")
	}
	if c.Business.LuckyCodeMinLength < 1 || c.Business.LuckyCodeMaxLength < c.Business.LuckyCodeMinLength {
		problems = append(problems, "business lucky code length range is invalid")
	}
	if c.Business.ActivityContentMaxLength < 1 {
		problems = append(problems, "business.activity_content_max_length must be positive")
	}
	if c.Business.FirstVisitTTL.Value() <= 0 || c.Business.LuckyClaimTTL.Value() <= 0 {
		problems = append(problems, "business ttl values must be positive")
	}
	if clock := c.Business.DailyResetClock; clock.Hour < 0 || clock.Hour > 23 || clock.Minute < 0 || clock.Minute > 59 {
		problems = append(problems, "business.daily_reset_time must be a valid HH:MM clock time")
	}
	if !adminPhonePattern.MatchString(strings.TrimSpace(c.Business.AdminPhone)) {
		problems = append(problems, "business.admin_phone must be a valid 11-digit mainland mobile number")
	}
	if strings.TrimSpace(c.Business.QRCodeUploadDir) == "" {
		problems = append(problems, "business.qrcode_upload_dir is required")
	}
	if c.Business.QRCodeMaxUploadBytes < 1024 || c.Business.QRCodeMaxUploadBytes > 20*1024*1024 {
		problems = append(problems, "business.qrcode_max_upload_bytes must be between 1024 and 20971520")
	}
	if c.Security.AdminSessionTTL.Value() <= 0 {
		problems = append(problems, "security.admin_session_ttl must be positive")
	}
	if strings.TrimSpace(c.Security.AdminTokenSecret) == "" {
		problems = append(problems, "security.admin_token_secret is required")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func applyEnvironment(c *Config) error {
	setString("APP_SERVER_HOST", &c.Server.Host)
	setString("APP_SERVER_MODE", &c.Server.Mode)
	setString("APP_MYSQL_DSN", &c.MySQL.DSN)
	setString("APP_REDIS_ADDRESS", &c.Redis.Address)
	setString("APP_REDIS_PASSWORD", &c.Redis.Password)
	setString("APP_ADMIN_PHONE", &c.Business.AdminPhone)
	setString("APP_QRCODE_UPLOAD_DIR", &c.Business.QRCodeUploadDir)
	setString("APP_ADMIN_TOKEN_SECRET", &c.Security.AdminTokenSecret)
	if value, ok := os.LookupEnv("APP_QRCODE_MAX_UPLOAD_BYTES"); ok {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("parse APP_QRCODE_MAX_UPLOAD_BYTES: %w", err)
		}
		c.Business.QRCodeMaxUploadBytes = parsed
	}

	for name, target := range map[string]*int{
		"APP_SERVER_PORT":                &c.Server.Port,
		"APP_REDIS_DATABASE":             &c.Redis.Database,
		"APP_MYSQL_MAX_IDLE_CONNECTIONS": &c.MySQL.MaxIdleConnections,
		"APP_MYSQL_MAX_OPEN_CONNECTIONS": &c.MySQL.MaxOpenConnections,
	} {
		if value, ok := os.LookupEnv(name); ok {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("parse %s: %w", name, err)
			}
			*target = parsed
		}
	}
	return nil
}

func setString(name string, target *string) {
	if value, ok := os.LookupEnv(name); ok {
		*target = value
	}
}
