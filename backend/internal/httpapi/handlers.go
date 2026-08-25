package httpapi

import (
	"net/http"
	"strconv"

	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/gin-gonic/gin"
)

func (s *Server) live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) ready(c *gin.Context) {
	ctx := c.Request.Context()
	sqlDB, err := s.db.DB()
	if err != nil || sqlDB.PingContext(ctx) != nil || s.redis.Ping(ctx).Err() != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func (s *Server) userInfo(c *gin.Context) {
	result, err := s.platform.UserInfo(c.Request.Context(), uid(c))
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) bindPhone(c *gin.Context) {
	var request struct {
		Phone string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.BindPhone(c.Request.Context(), uid(c), request.Phone)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) applyReferral(c *gin.Context) {
	var request struct {
		Phone string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.ApplyReferral(c.Request.Context(), uid(c), request.Phone)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) publishLucky(c *gin.Context) {
	var request struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.PublishLucky(c.Request.Context(), uid(c), request.Code)
	if err != nil {
		fail(c, err)
		return
	}
	created(c, result)
}

func (s *Server) listLucky(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := s.platform.ListLucky(c.Request.Context(), uid(c), limit)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, gin.H{"items": result, "count": len(result)})
}

func (s *Server) luckyStats(c *gin.Context) {
	result, err := s.platform.LuckyStats(c.Request.Context(), uid(c))
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) receiveLucky(c *gin.Context) {
	result, err := s.platform.ReceiveLucky(c.Request.Context(), uid(c))
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) useLucky(c *gin.Context) {
	var request struct {
		ID uint `json:"id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.UseLucky(c.Request.Context(), uid(c), request.ID)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) publishActivity(c *gin.Context) {
	var request struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.PublishActivity(c.Request.Context(), uid(c), request.Type, request.Content)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) activityDetail(c *gin.Context) {
	result, err := s.platform.ActivityDetail(c.Request.Context(), uid(c), c.Query("type"))
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) boostActivity(c *gin.Context) {
	var request struct {
		Type   string `json:"type"`
		Points int64  `json:"points"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.BoostActivity(c.Request.Context(), uid(c), request.Type, request.Points)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) useActivity(c *gin.Context) {
	var request struct {
		Type string `json:"type"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.UseActivity(c.Request.Context(), uid(c), request.Type)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) points(c *gin.Context) {
	result, err := s.platform.Points(c.Request.Context(), uid(c))
	if err != nil {
		fail(c, err)
		return
	}
	success(c, gin.H{"points": result})
}

func (s *Server) pointsHistory(c *gin.Context) {
	limit, offset := parsePagination(c)
	result, err := s.platform.PointsHistory(c.Request.Context(), uid(c), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, gin.H{"items": result})
}

func (s *Server) exchange(c *gin.Context) {
	var request struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.Exchange(c.Request.Context(), uid(c), request.Code)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) notice(c *gin.Context) {
	result, err := s.platform.Notice(c.Request.Context(), c.Param("type"))
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) groupQRCode(c *gin.Context) {
	result, err := s.platform.GroupQRCode(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	success(c, gin.H{
		"qrcode": uploadedQRCodeURL(result), "max_upload_bytes": s.qrcodeMaxUploadBytes,
	})
}
