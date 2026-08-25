package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadUsesFileAndEnvironmentOverrides(t *testing.T) {
	t.Setenv("APP_SERVER_PORT", "9090")
	t.Setenv("APP_REDIS_ADDRESS", "redis.internal:6379")
	t.Setenv("APP_QRCODE_UPLOAD_DIR", "custom-uploads")
	t.Setenv("APP_QRCODE_MAX_UPLOAD_BYTES", "1048576")

	path := filepath.Join(t.TempDir(), "config.yaml")
	err := os.WriteFile(path, []byte(`
server:
  mode: test
mysql:
  dsn: user:pass@tcp(mysql:3306)/test
redis:
  address: localhost:6379
business:
  admin_phone: "13800000000"
security:
  admin_token_secret: test-secret
`), 0o600)
	require.NoError(t, err)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, 9090, cfg.Server.Port)
	require.Equal(t, "redis.internal:6379", cfg.Redis.Address)
	require.Equal(t, 200, cfg.Business.ActivityContentMaxLength)
	require.Equal(t, "custom-uploads", cfg.Business.QRCodeUploadDir)
	require.EqualValues(t, 1048576, cfg.Business.QRCodeMaxUploadBytes)
}

func TestValidateRejectsMissingInfrastructure(t *testing.T) {
	cfg := defaults()
	cfg.MySQL.DSN = ""
	cfg.Business.AdminPhone = ""
	cfg.Security.AdminTokenSecret = ""

	err := cfg.Validate()
	require.ErrorContains(t, err, "mysql.dsn is required")
	require.ErrorContains(t, err, "business.admin_phone must be a valid 11-digit mainland mobile number")
}

func TestValidateRejectsInvalidAdminPhone(t *testing.T) {
	cfg := defaults()
	cfg.MySQL.DSN = "user:pass@tcp(mysql:3306)/test"
	cfg.Business.AdminPhone = "admin"
	cfg.Security.AdminTokenSecret = "test-secret"

	err := cfg.Validate()
	require.ErrorContains(t, err, "business.admin_phone must be a valid 11-digit mainland mobile number")
}

func TestValidateRejectsInvalidQRCodeUploadSettings(t *testing.T) {
	cfg := defaults()
	cfg.MySQL.DSN = "user:pass@tcp(mysql:3306)/test"
	cfg.Business.AdminPhone = "13800000000"
	cfg.Business.QRCodeUploadDir = ""
	cfg.Business.QRCodeMaxUploadBytes = 512
	cfg.Security.AdminTokenSecret = "test-secret"

	err := cfg.Validate()
	require.ErrorContains(t, err, "business.qrcode_upload_dir is required")
	require.ErrorContains(t, err, "business.qrcode_max_upload_bytes")
}
