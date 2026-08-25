package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/eaok-cn/kuaizudui/backend/internal/config"
	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/eaok-cn/kuaizudui/backend/internal/platform"
	"github.com/eaok-cn/kuaizudui/backend/internal/realtime"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	uidContextKey        = "uid"
	adminPhoneContextKey = "admin_phone"
	adminTokenContextKey = "admin_token"
)

type Server struct {
	platform             *platform.Platform
	db                   *gorm.DB
	redis                redis.UniversalClient
	activityUpdates      *realtime.ActivityUpdates
	qrcodeUploadDir      string
	qrcodeMaxUploadBytes int64
	qrcodeUploadMu       sync.Mutex
}

func NewRouter(app *platform.Platform, db *gorm.DB, redisClient redis.UniversalClient, cfg config.Config) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)
	uploadDir := strings.TrimSpace(cfg.Business.QRCodeUploadDir)
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	maxUploadBytes := cfg.Business.QRCodeMaxUploadBytes
	if maxUploadBytes <= 0 {
		maxUploadBytes = 5 * 1024 * 1024
	}
	server := &Server{
		platform: app, db: db, redis: redisClient, activityUpdates: realtime.NewActivityUpdates(redisClient),
		qrcodeUploadDir: uploadDir, qrcodeMaxUploadBytes: maxUploadBytes,
	}
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), cors(cfg.Server.AllowedOrigins))

	router.GET("/health/live", server.live)
	router.GET("/health/ready", server.ready)

	v1 := router.Group("/api/v1")
	v1.Use(identity())
	{
		v1.GET("/user/info", server.userInfo)
		v1.POST("/user/bind-phone", server.bindPhone)
		v1.POST("/user/referral", server.applyReferral)

		v1.POST("/lucky/publish", server.publishLucky)
		v1.GET("/lucky/list", server.listLucky)
		v1.GET("/lucky/stats", server.luckyStats)
		v1.POST("/lucky/receive", server.receiveLucky)
		v1.POST("/lucky/use", server.useLucky)

		v1.POST("/activity/publish", server.publishActivity)
		v1.GET("/activity/detail", server.activityDetail)
		v1.POST("/activity/boost", server.boostActivity)
		v1.POST("/activity/use", server.useActivity)
		v1.GET("/activity/events", server.activityEvents)

		v1.GET("/points", server.points)
		v1.GET("/points/history", server.pointsHistory)
		v1.POST("/exchange", server.exchange)
		v1.GET("/notices/:type", server.notice)
		v1.GET("/group-qrcode", server.groupQRCode)
		v1.GET("/group-qrcode/image", server.groupQRCodeImage)
		v1.POST("/feedback", server.submitFeedback)

		v1.POST("/admin/login", server.adminLogin)
		admin := v1.Group("/admin")
		admin.Use(server.adminAuth())
		{
			admin.POST("/logout", server.adminLogout)
			admin.GET("/users", server.adminListUsers)
			admin.POST("/recharge", server.adminRecharge)
			admin.GET("/recharges", server.adminListRecharges)
			admin.POST("/notice", server.adminSetNotice)
			admin.POST("/exchange/create", server.adminCreateExchangeCodes)
			admin.GET("/exchanges", server.adminListExchangeCodes)
			admin.POST("/qrcode", server.adminSetGroupQRCode)
			admin.DELETE("/qrcode", server.adminRemoveGroupQRCode)
			admin.GET("/feedback", server.adminListFeedback)
		}
	}
	return router
}

func identity() gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := strings.TrimSpace(c.GetHeader("X-UID"))
		if uid == "" {
			generated, err := platform.GenerateUID()
			if err != nil {
				fail(c, err)
				c.Abort()
				return
			}
			uid = generated
		}
		c.Header("X-UID", uid)
		c.Set(uidContextKey, uid)
		c.Next()
	}
}

func cors(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Expose-Headers", "X-UID")
		}
		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-UID")
			c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) adminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		phone, err := s.platform.AuthenticateAdmin(c.Request.Context(), token)
		if err != nil {
			fail(c, err)
			c.Abort()
			return
		}
		c.Set(adminPhoneContextKey, phone)
		c.Set(adminTokenContextKey, token)
		c.Next()
	}
}

func bearerToken(header string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func uid(c *gin.Context) string        { return c.GetString(uidContextKey) }
func adminPhone(c *gin.Context) string { return c.GetString(adminPhoneContextKey) }
func adminToken(c *gin.Context) string { return c.GetString(adminTokenContextKey) }

func success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

func created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{"code": 0, "message": "ok", "data": data})
}

func fail(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "internal server error"
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		status, code, message = http.StatusBadRequest, "invalid_input", err.Error()
	case errors.Is(err, domain.ErrUnauthorized):
		status, code, message = http.StatusUnauthorized, "unauthorized", "unauthorized"
	case errors.Is(err, domain.ErrForbidden), errors.Is(err, domain.ErrCannotUseOwn):
		status, code, message = http.StatusForbidden, "forbidden", err.Error()
	case errors.Is(err, domain.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "resource not found"
	case errors.Is(err, domain.ErrQueueEmpty):
		status, code, message = http.StatusNotFound, "queue_empty", "no available lucky code"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrAlreadyUsed):
		status, code, message = http.StatusConflict, "conflict", err.Error()
	case errors.Is(err, domain.ErrInsufficientPoints):
		status, code, message = http.StatusUnprocessableEntity, "insufficient_points", err.Error()
	default:
		_ = c.Error(err)
	}
	c.JSON(status, gin.H{"code": code, "message": message})
}

func parsePagination(c *gin.Context) (int, int) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	return limit, offset
}
