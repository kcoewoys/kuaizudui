package httpapi

import (
	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/gin-gonic/gin"
)

func (s *Server) adminLogin(c *gin.Context) {
	var request struct {
		Phone string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.AdminLogin(c.Request.Context(), request.Phone)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) adminLogout(c *gin.Context) {
	if err := s.platform.AdminLogout(c.Request.Context(), adminToken(c)); err != nil {
		fail(c, err)
		return
	}
	success(c, gin.H{"logged_out": true})
}

func (s *Server) adminListUsers(c *gin.Context) {
	limit, offset := parsePagination(c)
	result, err := s.platform.AdminListUsers(c.Request.Context(), c.Query("q"), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) adminRecharge(c *gin.Context) {
	var request struct {
		Phone  string `json:"phone"`
		Points int64  `json:"points"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.AdminRecharge(c.Request.Context(), adminPhone(c), request.Phone, request.Points)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) adminListRecharges(c *gin.Context) {
	limit, offset := parsePagination(c)
	result, err := s.platform.AdminListRecharges(c.Request.Context(), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, gin.H{"items": result})
}

func (s *Server) adminSetNotice(c *gin.Context) {
	var request struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.AdminSetNotice(c.Request.Context(), request.Type, request.Content)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, result)
}

func (s *Server) adminCreateExchangeCodes(c *gin.Context) {
	var request struct {
		Points int64  `json:"points"`
		Count  int    `json:"count"`
		Prefix string `json:"prefix"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.AdminCreateExchangeCodes(c.Request.Context(), request.Points, request.Count, request.Prefix)
	if err != nil {
		fail(c, err)
		return
	}
	created(c, gin.H{"items": result, "count": len(result)})
}

func (s *Server) adminListExchangeCodes(c *gin.Context) {
	limit, offset := parsePagination(c)
	result, err := s.platform.AdminListExchangeCodes(c.Request.Context(), c.Query("status"), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, gin.H{"items": result})
}
