package platform

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const adminSessionPrefix = "admin_session:"

type AdminLoginResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type UserPage struct {
	Items  []domain.User `json:"items"`
	Total  int64         `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

func (p *Platform) AdminLogin(ctx context.Context, phone string) (AdminLoginResult, error) {
	phone = strings.TrimSpace(phone)
	if phone != p.business.AdminPhone {
		return AdminLoginResult{}, domain.ErrUnauthorized
	}
	token, err := p.newAdminToken()
	if err != nil {
		return AdminLoginResult{}, err
	}
	ttl := p.security.AdminSessionTTL.Value()
	if err := p.redis.Set(ctx, adminSessionPrefix+token, phone, ttl).Err(); err != nil {
		return AdminLoginResult{}, fmt.Errorf("store admin session: %w", err)
	}
	return AdminLoginResult{Token: token, ExpiresAt: p.now().Add(ttl)}, nil
}

func (p *Platform) AuthenticateAdmin(ctx context.Context, token string) (string, error) {
	if !p.validAdminToken(token) {
		return "", domain.ErrUnauthorized
	}
	phone, err := p.redis.Get(ctx, adminSessionPrefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return "", domain.ErrUnauthorized
	}
	if err != nil {
		return "", fmt.Errorf("load admin session: %w", err)
	}
	return phone, nil
}

func (p *Platform) AdminLogout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := p.redis.Del(ctx, adminSessionPrefix+token).Err(); err != nil {
		return fmt.Errorf("delete admin session: %w", err)
	}
	return nil
}

func (p *Platform) AdminRecharge(ctx context.Context, adminPhone, phone string, points int64) (domain.User, error) {
	phone = strings.TrimSpace(phone)
	if !phonePattern.MatchString(phone) {
		return domain.User{}, domain.FieldError{Field: "phone", Message: "must be a valid 11-digit mainland mobile number"}
	}
	if points <= 0 || points > 1_000_000 {
		return domain.User{}, domain.FieldError{Field: "points", Message: "must be between 1 and 1000000"}
	}
	var user domain.User
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("phone = ?", phone).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := tx.Model(&user).Update("points", gorm.Expr("points + ?", points)).Error; err != nil {
			return err
		}
		record := domain.RechargeRecord{Phone: phone, Points: points, AdminPhone: adminPhone}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		if err := tx.Create(&domain.PointRecord{
			UID: user.UID, Source: "admin_recharge", Description: "运营充值", Points: points,
		}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", user.ID).First(&user).Error
	})
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (p *Platform) AdminSetNotice(ctx context.Context, noticeType, content string) (domain.Notice, error) {
	noticeType = strings.TrimSpace(noticeType)
	content = strings.TrimSpace(content)
	if noticeType == "" || len(noticeType) > 50 {
		return domain.Notice{}, domain.FieldError{Field: "type", Message: "must contain 1 to 50 characters"}
	}
	if len(content) > 10_000 {
		return domain.Notice{}, domain.FieldError{Field: "content", Message: "must not exceed 10000 characters"}
	}
	notice := domain.Notice{Type: noticeType, Content: content, UpdatedAt: p.now()}
	if err := p.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "type"}},
		DoUpdates: clause.Assignments(map[string]any{"content": content, "updated_at": notice.UpdatedAt}),
	}).Create(&notice).Error; err != nil {
		return domain.Notice{}, fmt.Errorf("set notice: %w", err)
	}
	return notice, nil
}

func (p *Platform) AdminSetGroupQRCode(ctx context.Context, value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 2_000 {
		return "", domain.FieldError{Field: "qrcode", Message: "must not exceed 2000 characters"}
	}
	setting := domain.Setting{Key: groupQRCodeKey, Value: value, UpdatedAt: p.now()}
	if err := p.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]any{"value": value, "updated_at": setting.UpdatedAt}),
	}).Create(&setting).Error; err != nil {
		return "", fmt.Errorf("set group qrcode: %w", err)
	}
	return value, nil
}

func (p *Platform) AdminCreateExchangeCodes(ctx context.Context, points int64, count int, prefix string) ([]domain.ExchangeCode, error) {
	if points <= 0 || points > 1_000_000 {
		return nil, domain.FieldError{Field: "points", Message: "must be between 1 and 1000000"}
	}
	if count < 1 || count > 500 {
		return nil, domain.FieldError{Field: "count", Message: "must be between 1 and 500"}
	}
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if len(prefix) > 12 {
		return nil, domain.FieldError{Field: "prefix", Message: "must not exceed 12 characters"}
	}
	created := make([]domain.ExchangeCode, 0, count)
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for attempts := 0; len(created) < count && attempts < count*5; attempts++ {
			suffix, err := randomReadableCode(12)
			if err != nil {
				return err
			}
			code := domain.ExchangeCode{
				Code: prefix + suffix, Points: points, Status: domain.ExchangeStatusUnused,
			}
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&code)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				created = append(created, code)
			}
		}
		if len(created) != count {
			return fmt.Errorf("could not generate %d unique exchange codes", count)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create exchange codes: %w", err)
	}
	return created, nil
}

func (p *Platform) AdminListUsers(ctx context.Context, query string, limit, offset int) (UserPage, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	db := p.db.WithContext(ctx).Model(&domain.User{})
	query = strings.TrimSpace(query)
	if query != "" {
		like := "%" + query + "%"
		db = db.Where("uid LIKE ? OR phone LIKE ?", like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return UserPage{}, fmt.Errorf("count users: %w", err)
	}
	var users []domain.User
	if err := db.Order("id DESC").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return UserPage{}, fmt.Errorf("list users: %w", err)
	}
	return UserPage{Items: users, Total: total, Limit: limit, Offset: offset}, nil
}

func (p *Platform) AdminListRecharges(ctx context.Context, limit, offset int) ([]domain.RechargeRecord, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var records []domain.RechargeRecord
	if err := p.db.WithContext(ctx).Order("id DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list recharge records: %w", err)
	}
	return records, nil
}

func (p *Platform) AdminListExchangeCodes(ctx context.Context, status string, limit, offset int) ([]domain.ExchangeCode, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	db := p.db.WithContext(ctx).Model(&domain.ExchangeCode{})
	if status != "" {
		if status != domain.ExchangeStatusUnused && status != domain.ExchangeStatusUsed {
			return nil, domain.FieldError{Field: "status", Message: "must be unused or used"}
		}
		db = db.Where("status = ?", status)
	}
	var codes []domain.ExchangeCode
	if err := db.Order("id DESC").Limit(limit).Offset(offset).Find(&codes).Error; err != nil {
		return nil, fmt.Errorf("list exchange codes: %w", err)
	}
	return codes, nil
}

func (p *Platform) newAdminToken() (string, error) {
	payload := make([]byte, 24)
	if _, err := rand.Read(payload); err != nil {
		return "", fmt.Errorf("generate admin token: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := p.signToken(encoded)
	return encoded + "." + signature, nil
}

func (p *Platform) validAdminToken(token string) bool {
	payload, signature, ok := strings.Cut(token, ".")
	if !ok || payload == "" || signature == "" {
		return false
	}
	expected := p.signToken(payload)
	return hmac.Equal([]byte(signature), []byte(expected))
}

func (p *Platform) signToken(payload string) string {
	mac := hmac.New(sha256.New, []byte(p.security.AdminTokenSecret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomReadableCode(length int) (string, error) {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate exchange code: %w", err)
	}
	for index, value := range raw {
		raw[index] = alphabet[int(value)%len(alphabet)]
	}
	return string(raw), nil
}
