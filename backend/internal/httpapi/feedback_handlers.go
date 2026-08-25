package httpapi

import (
	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/gin-gonic/gin"
)

func (s *Server) submitFeedback(c *gin.Context) {
	var request struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, domain.FieldError{Field: "body", Message: "invalid JSON"})
		return
	}
	result, err := s.platform.SubmitFeedback(c.Request.Context(), uid(c), request.Content)
	if err != nil {
		fail(c, err)
		return
	}
	created(c, result)
}

func (s *Server) adminListFeedback(c *gin.Context) {
	limit, offset := parsePagination(c)
	result, err := s.platform.AdminListFeedback(c.Request.Context(), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	success(c, gin.H{"items": result})
}
