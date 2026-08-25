package platform

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
)

const maxFeedbackContentLength = 500

type AdminFeedbackItem struct {
	ID        uint      `json:"id"`
	UID       string    `json:"uid"`
	Phone     *string   `json:"phone,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func (p *Platform) SubmitFeedback(ctx context.Context, uid, content string) (domain.Feedback, error) {
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > maxFeedbackContentLength {
		return domain.Feedback{}, domain.FieldError{Field: "content", Message: "must contain 1 to 500 characters"}
	}
	if _, err := p.EnsureUser(ctx, uid); err != nil {
		return domain.Feedback{}, err
	}
	record := domain.Feedback{UID: uid, Content: content, CreatedAt: p.now()}
	if err := p.db.WithContext(ctx).Create(&record).Error; err != nil {
		return domain.Feedback{}, fmt.Errorf("create feedback: %w", err)
	}
	return record, nil
}

func (p *Platform) AdminListFeedback(ctx context.Context, limit, offset int) ([]AdminFeedbackItem, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var items []AdminFeedbackItem
	if err := p.db.WithContext(ctx).
		Table("feedbacks").
		Select("feedbacks.id, feedbacks.uid, users.phone, feedbacks.content, feedbacks.created_at").
		Joins("LEFT JOIN users ON users.uid = feedbacks.uid").
		Order("feedbacks.id DESC").
		Limit(limit).
		Offset(offset).
		Scan(&items).Error; err != nil {
		return nil, fmt.Errorf("list feedback: %w", err)
	}
	return items, nil
}
