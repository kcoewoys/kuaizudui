package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/gin-gonic/gin"
)

const qrcodeUploadMarker = "upload:"

var qrcodeImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
}

func (s *Server) adminSetGroupQRCode(c *gin.Context) {
	s.qrcodeUploadMu.Lock()
	defer s.qrcodeUploadMu.Unlock()

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.qrcodeMaxUploadBytes+1024*1024)
	header, err := c.FormFile("image")
	if err != nil {
		fail(c, domain.FieldError{Field: "image", Message: "a PNG or JPEG file is required"})
		return
	}
	if header.Size <= 0 || header.Size > s.qrcodeMaxUploadBytes {
		fail(c, domain.FieldError{Field: "image", Message: "file size exceeds the configured limit"})
		return
	}

	file, err := header.Open()
	if err != nil {
		fail(c, domain.FieldError{Field: "image", Message: "cannot read uploaded file"})
		return
	}
	contents, readErr := io.ReadAll(io.LimitReader(file, s.qrcodeMaxUploadBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		fail(c, domain.FieldError{Field: "image", Message: "cannot read uploaded file"})
		return
	}
	if int64(len(contents)) > s.qrcodeMaxUploadBytes {
		fail(c, domain.FieldError{Field: "image", Message: "file size exceeds the configured limit"})
		return
	}

	contentType := http.DetectContentType(contents)
	extension, allowed := qrcodeImageTypes[contentType]
	if !allowed {
		fail(c, domain.FieldError{Field: "image", Message: "only PNG and JPEG images are supported"})
		return
	}
	imageConfig, _, err := image.DecodeConfig(bytes.NewReader(contents))
	if err != nil || imageConfig.Width < 1 || imageConfig.Height < 1 || imageConfig.Width > 6000 || imageConfig.Height > 6000 {
		fail(c, domain.FieldError{Field: "image", Message: "image is invalid or its dimensions are too large"})
		return
	}

	if err := os.MkdirAll(s.qrcodeUploadDir, 0o750); err != nil {
		fail(c, err)
		return
	}
	name, err := newQRCodeUploadName(extension)
	if err != nil {
		fail(c, err)
		return
	}
	finalPath := filepath.Join(s.qrcodeUploadDir, name)
	temporary, err := os.CreateTemp(s.qrcodeUploadDir, ".qrcode-upload-*")
	if err != nil {
		fail(c, err)
		return
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		fail(c, err)
		return
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		fail(c, err)
		return
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		fail(c, err)
		return
	}
	if err := temporary.Close(); err != nil {
		fail(c, err)
		return
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		fail(c, err)
		return
	}

	previous, _ := s.platform.GroupQRCode(c.Request.Context())
	storedValue := qrcodeUploadMarker + name
	if _, err := s.platform.AdminSetGroupQRCode(c.Request.Context(), storedValue); err != nil {
		_ = os.Remove(finalPath)
		fail(c, err)
		return
	}
	removeUploadedQRCode(s.qrcodeUploadDir, previous, name)
	success(c, gin.H{"qrcode": uploadedQRCodeURL(storedValue)})
}

func (s *Server) adminRemoveGroupQRCode(c *gin.Context) {
	s.qrcodeUploadMu.Lock()
	defer s.qrcodeUploadMu.Unlock()

	previous, err := s.platform.GroupQRCode(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	if _, err := s.platform.AdminSetGroupQRCode(c.Request.Context(), ""); err != nil {
		fail(c, err)
		return
	}
	removeUploadedQRCode(s.qrcodeUploadDir, previous, "")
	success(c, gin.H{"qrcode": ""})
}

func (s *Server) groupQRCodeImage(c *gin.Context) {
	storedValue, err := s.platform.GroupQRCode(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	name, ok := uploadedQRCodeName(storedValue)
	if !ok {
		fail(c, domain.ErrNotFound)
		return
	}
	path := filepath.Join(s.qrcodeUploadDir, name)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(c, domain.ErrNotFound)
			return
		}
		fail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.File(path)
}

func uploadedQRCodeURL(value string) string {
	name, ok := uploadedQRCodeName(value)
	if !ok {
		return value
	}
	return "/api/v1/group-qrcode/image?v=" + url.QueryEscape(name)
}

func uploadedQRCodeName(value string) (string, bool) {
	if !strings.HasPrefix(value, qrcodeUploadMarker) {
		return "", false
	}
	name := strings.TrimPrefix(value, qrcodeUploadMarker)
	if name == "" || filepath.Base(name) != name || strings.Contains(name, "..") {
		return "", false
	}
	extension := strings.ToLower(filepath.Ext(name))
	if extension != ".png" && extension != ".jpg" {
		return "", false
	}
	return name, true
}

func removeUploadedQRCode(directory, value, keepName string) {
	name, ok := uploadedQRCodeName(value)
	if !ok || name == keepName {
		return
	}
	_ = os.Remove(filepath.Join(directory, name))
}

func newQRCodeUploadName(extension string) (string, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "group-qrcode-" + hex.EncodeToString(random) + extension, nil
}
