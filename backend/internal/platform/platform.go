package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/eaok-cn/kuaizudui/backend/internal/config"
	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/eaok-cn/kuaizudui/backend/internal/queue"
	"github.com/eaok-cn/kuaizudui/backend/internal/realtime"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	firstVisitPrefix         = "first_visit:"
	groupQRCodeKey           = "group_qrcode"
	referralRewardPoints     = int64(10)
	maxActivityQueueAttempts = 1000
	dailyResetSettingKey     = "daily_reset.last_run"
	dailyResetDateLayout     = "2006-01-02"
)

var phonePattern = regexp.MustCompile(`^1\d{10}$`)

var errStaleActivityQueueEntry = errors.New("stale activity queue entry")
var errActivityAlreadyClaimed = errors.New("activity publisher already claimed by claimant")

type Platform struct {
	db       *gorm.DB
	redis    redis.UniversalClient
	lucky    *queue.LuckyQueue
	activity *queue.ActivityQueue
	updates  *realtime.ActivityUpdates
	business config.BusinessConfig
	security config.SecurityConfig
	now      func() time.Time
	resetMu  sync.Mutex
}

type UserInfo struct {
	UID            string  `json:"uid"`
	Phone          *string `json:"phone,omitempty"`
	InvitedByPhone *string `json:"invited_by_phone,omitempty"`
	Points         int64   `json:"points"`
	FirstVisit     bool    `json:"first_visit"`
}

type LuckyListItem struct {
	ID         uint      `json:"id"`
	MaskedCode string    `json:"masked_code"`
	Source     string    `json:"source"`
	IsOwn      bool      `json:"is_own"`
	CreatedAt  time.Time `json:"created_at"`
}

type LuckyResult struct {
	ID        uint      `json:"id"`
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"created_at"`
}

type LuckyStats struct {
	ClaimedToday   int64 `json:"claimed_today"`
	PublishedToday int64 `json:"published_today"`
}

type ActivityResult struct {
	Type            string     `json:"type"`
	Content         string     `json:"content"`
	Published       bool       `json:"published"`
	OrdinaryRounds  int64      `json:"ordinary_rounds"`
	OrdinaryCredit  int64      `json:"ordinary_credit"`
	PriorityRounds  int64      `json:"priority_rounds"`
	PointsCommitted int64      `json:"points_committed"`
	PriorityCredit  int64      `json:"priority_credit"`
	ClaimCount      int64      `json:"claim_count"`
	CanClaim        bool       `json:"can_claim"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

type ActivityUseResult struct {
	Content string         `json:"content"`
	Source  string         `json:"source"`
	State   ActivityResult `json:"state"`
}

type ExchangeResult struct {
	AwardedPoints int64 `json:"awarded_points"`
	TotalPoints   int64 `json:"total_points"`
}

func New(db *gorm.DB, redisClient redis.UniversalClient, cfg config.Config) *Platform {
	return &Platform{
		db: db, redis: redisClient,
		lucky:    queue.NewLuckyQueue(redisClient, cfg.Business.LuckyClaimTTL.Value()),
		activity: queue.NewActivityQueue(redisClient),
		updates:  realtime.NewActivityUpdates(redisClient),
		business: cfg.Business, security: cfg.Security, now: time.Now,
	}
}

func (p *Platform) ResetActivityQueues(ctx context.Context) error {
	for activityType := range domain.ValidActivityTypes {
		if err := p.activity.Reset(ctx, activityType); err != nil {
			return err
		}
	}
	return nil
}

// ResetDailyData wipes everything Redis holds for this deployment and empties
// the daily MySQL tables (activity contents, activity claims, lucky codes).
// Durable data — users, points, point records, recharge records, exchange
// codes, notices, settings, feedback — is left untouched. Redis goes first so
// an interrupted reset leaves the activity queues re-seeding from MySQL
// instead of serving already-deleted entries.
func (p *Platform) ResetDailyData(ctx context.Context) error {
	if err := p.redis.FlushDB(ctx).Err(); err != nil {
		return fmt.Errorf("flush redis for daily reset: %w", err)
	}
	if err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, target := range []any{&domain.ActivityClaim{}, &domain.ActivityContent{}, &domain.LuckyCode{}} {
			if err := tx.Where("1 = 1").Delete(target).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("empty daily tables: %w", err)
	}
	return nil
}

// ResetDailyDataIfDue performs the daily reset once today's reset time has
// passed and no reset has been recorded for today. The completion marker lives
// in MySQL, so restarting the server never wipes the same day twice and a
// reset missed while the server was down is caught up on the next start.
func (p *Platform) ResetDailyDataIfDue(ctx context.Context) (bool, error) {
	p.resetMu.Lock()
	defer p.resetMu.Unlock()

	now := p.now()
	if now.Before(clockOnDate(now, p.business.DailyResetClock)) {
		return false, nil
	}
	today := now.Format(dailyResetDateLayout)
	lastRun, err := p.dailyResetLastRun(ctx)
	if err != nil {
		return false, err
	}
	if lastRun == today {
		return false, nil
	}
	if err := p.ResetDailyData(ctx); err != nil {
		return false, err
	}
	if err := p.markDailyResetDone(ctx, today); err != nil {
		return false, err
	}
	return true, nil
}

// RunDailyResetScheduler blocks until ctx is cancelled, resetting the daily
// data whenever it becomes due; a failed attempt retries every minute so a
// transient error cannot skip the day.
func (p *Platform) RunDailyResetScheduler(ctx context.Context) error {
	const retryInterval = time.Minute
	for {
		ran, err := p.ResetDailyDataIfDue(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "daily data reset failed, will retry", "error", err)
			if !waitFor(ctx, retryInterval) {
				return ctx.Err()
			}
			continue
		}
		if ran {
			slog.InfoContext(ctx, "daily data reset completed")
		}
		if !waitFor(ctx, time.Until(p.nextDailyReset(p.now()))) {
			return ctx.Err()
		}
	}
}

func waitFor(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (p *Platform) nextDailyReset(now time.Time) time.Time {
	next := clockOnDate(now, p.business.DailyResetClock)
	if !now.Before(next) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func clockOnDate(day time.Time, clock config.Clock) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), clock.Hour, clock.Minute, 0, 0, day.Location())
}

func (p *Platform) dailyResetLastRun(ctx context.Context) (string, error) {
	var marker domain.Setting
	err := p.db.WithContext(ctx).Where("`key` = ?", dailyResetSettingKey).First(&marker).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load daily reset marker: %w", err)
	}
	return marker.Value, nil
}

func (p *Platform) markDailyResetDone(ctx context.Context, date string) error {
	marker := domain.Setting{Key: dailyResetSettingKey, Value: date}
	if err := p.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&marker).Error; err != nil {
		return fmt.Errorf("store daily reset marker: %w", err)
	}
	return nil
}

func GenerateUID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate uid: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func (p *Platform) EnsureUser(ctx context.Context, uid string) (domain.User, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" || len(uid) > 40 {
		return domain.User{}, domain.FieldError{Field: "uid", Message: "must contain 1 to 40 characters"}
	}
	user := domain.User{UID: uid}
	if err := p.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&user).Error; err != nil {
		return domain.User{}, fmt.Errorf("ensure user: %w", err)
	}
	if err := p.db.WithContext(ctx).Where("uid = ?", uid).First(&user).Error; err != nil {
		return domain.User{}, fmt.Errorf("load user: %w", err)
	}
	return user, nil
}

func (p *Platform) UserInfo(ctx context.Context, uid string) (UserInfo, error) {
	user, err := p.EnsureUser(ctx, uid)
	if err != nil {
		return UserInfo{}, err
	}
	firstVisit, err := p.redis.SetNX(ctx, firstVisitPrefix+uid, "1", p.business.FirstVisitTTL.Value()).Result()
	if err != nil {
		return UserInfo{}, fmt.Errorf("record first visit: %w", err)
	}
	result, err := p.userInfoFromUser(ctx, user)
	if err != nil {
		return UserInfo{}, err
	}
	result.FirstVisit = firstVisit
	return result, nil
}

func (p *Platform) BindPhone(ctx context.Context, uid, phone string) (UserInfo, error) {
	phone = strings.TrimSpace(phone)
	if !phonePattern.MatchString(phone) {
		return UserInfo{}, domain.FieldError{Field: "phone", Message: "must be a valid 11-digit mainland mobile number"}
	}
	if _, err := p.EnsureUser(ctx, uid); err != nil {
		return UserInfo{}, err
	}
	var user domain.User
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uid = ?", uid).First(&user).Error; err != nil {
			return err
		}
		if user.Phone != nil {
			if *user.Phone != phone {
				return fmt.Errorf("%w: phone is already bound", domain.ErrConflict)
			}
			return nil
		}
		var existing int64
		if err := tx.Model(&domain.User{}).Where("phone = ? AND uid <> ?", phone, uid).Count(&existing).Error; err != nil {
			return fmt.Errorf("check phone binding: %w", err)
		}
		if existing > 0 {
			return fmt.Errorf("%w: phone already bound", domain.ErrConflict)
		}
		if err := tx.Model(&user).Update("phone", phone).Error; err != nil {
			return fmt.Errorf("bind phone: %w", err)
		}
		if user.InvitedByUID == nil || strings.TrimSpace(*user.InvitedByUID) == "" {
			return tx.Where("uid = ?", uid).First(&user).Error
		}
		if err := tx.Model(&domain.User{}).Where("uid = ?", uid).Update("points", gorm.Expr("points + ?", referralRewardPoints)).Error; err != nil {
			return fmt.Errorf("award referred user points: %w", err)
		}
		if err := tx.Model(&domain.User{}).Where("uid = ?", *user.InvitedByUID).Update("points", gorm.Expr("points + ?", referralRewardPoints)).Error; err != nil {
			return fmt.Errorf("award inviter points: %w", err)
		}
		if err := tx.Create(&domain.PointRecord{
			UID: uid, Source: "invite", Description: "好友绑定手机号奖励", Points: referralRewardPoints,
		}).Error; err != nil {
			return fmt.Errorf("record referred user points: %w", err)
		}
		if err := tx.Create(&domain.PointRecord{
			UID: *user.InvitedByUID, Source: "invite", Description: "好友绑定手机号奖励", Points: referralRewardPoints,
		}).Error; err != nil {
			return fmt.Errorf("record inviter points: %w", err)
		}
		return tx.Where("uid = ?", uid).First(&user).Error
	})
	if err != nil {
		return UserInfo{}, err
	}
	return p.userInfoFromUser(ctx, user)
}

func (p *Platform) ApplyReferral(ctx context.Context, uid, inviterPhone string) (UserInfo, error) {
	inviterPhone = strings.TrimSpace(inviterPhone)
	if !phonePattern.MatchString(inviterPhone) {
		return UserInfo{}, domain.FieldError{Field: "phone", Message: "must be a valid 11-digit mainland mobile number"}
	}
	user, err := p.EnsureUser(ctx, uid)
	if err != nil {
		return UserInfo{}, err
	}
	if user.Phone != nil {
		return p.userInfoFromUser(ctx, user)
	}
	var inviter domain.User
	if err := p.db.WithContext(ctx).Where("phone = ?", inviterPhone).First(&inviter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return UserInfo{}, domain.ErrNotFound
		}
		return UserInfo{}, fmt.Errorf("find inviter: %w", err)
	}
	if inviter.UID == user.UID {
		return UserInfo{}, fmt.Errorf("%w: cannot invite yourself", domain.ErrConflict)
	}

	err = p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current domain.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uid = ?", uid).First(&current).Error; err != nil {
			return err
		}
		if current.InvitedByUID != nil && strings.TrimSpace(*current.InvitedByUID) != "" {
			return nil
		}
		return tx.Model(&current).Update("invited_by_uid", inviter.UID).Error
	})
	if err != nil {
		return UserInfo{}, fmt.Errorf("apply referral: %w", err)
	}
	if err := p.db.WithContext(ctx).Where("uid = ?", uid).First(&user).Error; err != nil {
		return UserInfo{}, fmt.Errorf("load referred user: %w", err)
	}
	return p.userInfoFromUser(ctx, user)
}

func (p *Platform) userInfoFromUser(ctx context.Context, user domain.User) (UserInfo, error) {
	result := UserInfo{UID: user.UID, Phone: user.Phone, Points: user.Points}
	if user.InvitedByUID == nil || strings.TrimSpace(*user.InvitedByUID) == "" {
		return result, nil
	}
	var inviter domain.User
	if err := p.db.WithContext(ctx).Select("phone").Where("uid = ?", *user.InvitedByUID).First(&inviter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, nil
		}
		return UserInfo{}, fmt.Errorf("load inviter phone: %w", err)
	}
	result.InvitedByPhone = inviter.Phone
	return result, nil
}

func (p *Platform) PublishLucky(ctx context.Context, uid, code string) (LuckyResult, error) {
	code = strings.TrimSpace(code)
	if err := validateDigits(code, p.business.LuckyCodeMinLength, p.business.LuckyCodeMaxLength); err != nil {
		return LuckyResult{}, err
	}
	if _, err := p.EnsureUser(ctx, uid); err != nil {
		return LuckyResult{}, err
	}
	item := domain.LuckyCode{UID: uid, Code: code, Status: domain.LuckyStatusAvailable}
	result := p.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
	if result.Error != nil {
		return LuckyResult{}, fmt.Errorf("create lucky code: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return LuckyResult{}, fmt.Errorf("%w: lucky code already published", domain.ErrConflict)
	}
	entry := queue.LuckyEntry{ID: item.ID, UID: uid}
	if err := p.lucky.Publish(ctx, entry); err != nil {
		_ = p.db.WithContext(ctx).Delete(&item).Error
		return LuckyResult{}, err
	}
	return LuckyResult{ID: item.ID, Code: item.Code, CreatedAt: item.CreatedAt}, nil
}

func (p *Platform) ListLucky(ctx context.Context, uid string, limit int) ([]LuckyListItem, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var codes []domain.LuckyCode
	if err := p.db.WithContext(ctx).
		Where("status = ?", domain.LuckyStatusAvailable).
		Order("id ASC").Limit(limit * 2).Find(&codes).Error; err != nil {
		return nil, fmt.Errorf("list lucky codes: %w", err)
	}
	items := make([]LuckyListItem, 0, limit)
	for _, code := range codes {
		claimed, err := p.lucky.Claimed(ctx, code.ID)
		if err != nil {
			return nil, err
		}
		if claimed {
			continue
		}
		isOwn := code.UID == uid
		source := maskIdentifier(code.UID)
		if isOwn {
			source = "我"
		}
		items = append(items, LuckyListItem{
			ID: code.ID, MaskedCode: maskCode(code.Code), Source: source, IsOwn: isOwn, CreatedAt: code.CreatedAt,
		})
		if len(items) == limit {
			break
		}
	}
	return items, nil
}

func (p *Platform) LuckyStats(ctx context.Context, uid string) (LuckyStats, error) {
	now := p.now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var stats LuckyStats
	if err := p.db.WithContext(ctx).Model(&domain.LuckyCode{}).
		Where("used_uid = ? AND used_at >= ?", uid, dayStart).
		Count(&stats.ClaimedToday).Error; err != nil {
		return LuckyStats{}, fmt.Errorf("count claimed lucky codes: %w", err)
	}
	if err := p.db.WithContext(ctx).Model(&domain.LuckyCode{}).
		Where("uid = ? AND created_at >= ?", uid, dayStart).
		Count(&stats.PublishedToday).Error; err != nil {
		return LuckyStats{}, fmt.Errorf("count published lucky codes: %w", err)
	}
	return stats, nil
}

func (p *Platform) ReceiveLucky(ctx context.Context, uid string) (LuckyResult, error) {
	if _, err := p.EnsureUser(ctx, uid); err != nil {
		return LuckyResult{}, err
	}
	for attempt := 0; attempt < 10; attempt++ {
		entry, err := p.lucky.Receive(ctx, uid)
		if err != nil {
			return LuckyResult{}, err
		}
		var item domain.LuckyCode
		err = p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, entry.ID).Error; err != nil {
				return err
			}
			if item.Status != domain.LuckyStatusAvailable {
				return domain.ErrAlreadyUsed
			}
			if item.UID == uid {
				return domain.ErrCannotUseOwn
			}
			now := p.now()
			return tx.Model(&item).Updates(map[string]any{"status": domain.LuckyStatusUsed, "used_at": now, "used_uid": uid}).Error
		})
		if errors.Is(err, domain.ErrAlreadyUsed) || errors.Is(err, gorm.ErrRecordNotFound) {
			_ = p.lucky.Release(ctx, entry, false)
			continue
		}
		if errors.Is(err, domain.ErrCannotUseOwn) {
			_ = p.lucky.Release(ctx, entry, true)
			continue
		}
		if err != nil {
			_ = p.lucky.Release(ctx, entry, true)
			return LuckyResult{}, fmt.Errorf("receive lucky code transaction: %w", err)
		}
		return LuckyResult{ID: item.ID, Code: item.Code, CreatedAt: item.CreatedAt}, nil
	}
	return LuckyResult{}, domain.ErrQueueEmpty
}

func (p *Platform) UseLucky(ctx context.Context, uid string, id uint) (LuckyResult, error) {
	if id == 0 {
		return LuckyResult{}, domain.FieldError{Field: "id", Message: "must be positive"}
	}
	var item domain.LuckyCode
	if err := p.db.WithContext(ctx).First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return LuckyResult{}, domain.ErrNotFound
		}
		return LuckyResult{}, fmt.Errorf("load lucky code: %w", err)
	}
	if item.UID == uid {
		return LuckyResult{}, domain.ErrCannotUseOwn
	}
	if item.Status != domain.LuckyStatusAvailable {
		return LuckyResult{}, domain.ErrAlreadyUsed
	}
	entry := queue.LuckyEntry{ID: item.ID, UID: item.UID}
	claimed, err := p.lucky.Claim(ctx, item.ID, uid)
	if err != nil {
		return LuckyResult{}, err
	}
	if !claimed {
		return LuckyResult{}, domain.ErrAlreadyUsed
	}
	now := p.now()
	result := p.db.WithContext(ctx).Model(&domain.LuckyCode{}).
		Where("id = ? AND status = ?", id, domain.LuckyStatusAvailable).
		Updates(map[string]any{"status": domain.LuckyStatusUsed, "used_at": now, "used_uid": uid})
	if result.Error != nil {
		_ = p.lucky.Release(ctx, entry, false)
		return LuckyResult{}, fmt.Errorf("use lucky code: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		_ = p.lucky.Release(ctx, entry, false)
		return LuckyResult{}, domain.ErrAlreadyUsed
	}
	return LuckyResult{ID: item.ID, Code: item.Code, CreatedAt: item.CreatedAt}, nil
}

func (p *Platform) PublishActivity(ctx context.Context, uid, activityType, content string) (ActivityResult, error) {
	content = strings.TrimSpace(content)
	if _, ok := domain.ValidActivityTypes[activityType]; !ok {
		return ActivityResult{}, domain.FieldError{Field: "type", Message: "unsupported activity type"}
	}
	if content == "" || utf8.RuneCountInString(content) > p.business.ActivityContentMaxLength {
		return ActivityResult{}, domain.FieldError{Field: "content", Message: fmt.Sprintf("must contain 1 to %d characters", p.business.ActivityContentMaxLength)}
	}
	if _, err := p.EnsureUser(ctx, uid); err != nil {
		return ActivityResult{}, err
	}
	item := domain.ActivityContent{UID: uid, Type: activityType, Content: content, OrdinaryCredit: int64(p.business.ActivityPublishOrdinaryCredit)}
	result := p.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
	if result.Error != nil {
		return ActivityResult{}, fmt.Errorf("publish activity content: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Re-publishing only refreshes the content.
		if err := p.db.WithContext(ctx).Model(&domain.ActivityContent{}).
			Where("uid = ? AND type = ?", uid, activityType).
			Updates(map[string]any{"content": content, "updated_at": p.now()}).Error; err != nil {
			return ActivityResult{}, fmt.Errorf("update activity content: %w", err)
		}
	}
	if err := p.db.WithContext(ctx).Where("uid = ? AND type = ?", uid, activityType).First(&item).Error; err != nil {
		return ActivityResult{}, fmt.Errorf("load activity content: %w", err)
	}
	// A first publish enters the ordinary queue active with its granted
	// chances; re-publishing only refreshes the content and keeps every count,
	// so exhausted publishers are not refilled by editing.
	if err := p.activity.EnqueueOrdinary(ctx, activityType, uid, item.OrdinaryCredit); err != nil {
		_ = p.activity.InvalidateSeed(ctx, activityType)
		return ActivityResult{}, err
	}
	if item.PriorityCredit > 0 {
		if err := p.activity.EnqueuePriority(ctx, activityType, uid, item.PriorityCredit); err != nil {
			_ = p.activity.InvalidateSeed(ctx, activityType)
			return ActivityResult{}, err
		}
	}
	// A new participant (or fresh priority credit) reshapes every watcher's
	// availability, so the change is broadcast to the whole activity.
	p.broadcastActivityUpdate(ctx, activityType)
	return p.activityResultWithAvailability(ctx, uid, item)
}

func (p *Platform) ActivityDetail(ctx context.Context, uid, activityType string) (ActivityResult, error) {
	if _, ok := domain.ValidActivityTypes[activityType]; !ok {
		return ActivityResult{}, domain.FieldError{Field: "type", Message: "unsupported activity type"}
	}
	var item domain.ActivityContent
	err := p.db.WithContext(ctx).Where("uid = ? AND type = ?", uid, activityType).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ActivityResult{Type: activityType, Published: false}, nil
	}
	if err != nil {
		return ActivityResult{}, fmt.Errorf("load activity detail: %w", err)
	}
	return p.activityResultWithAvailability(ctx, uid, item)
}

func (p *Platform) BoostActivity(ctx context.Context, uid, activityType string, points int64) (ActivityResult, error) {
	if _, ok := domain.ValidActivityTypes[activityType]; !ok {
		return ActivityResult{}, domain.FieldError{Field: "type", Message: "unsupported activity type"}
	}
	if points < 1 {
		return ActivityResult{}, domain.FieldError{Field: "points", Message: "must be a positive integer"}
	}
	var item domain.ActivityContent
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user domain.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uid = ?", uid).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("uid = ? AND type = ?", uid, activityType).First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if user.Points < points {
			return domain.ErrInsufficientPoints
		}
		item.PointsCommitted += points
		item.PriorityCredit += points
		if err := tx.Model(&item).UpdateColumns(map[string]any{
			"boost_points_used": item.PointsCommitted,
			"priority_credit":   item.PriorityCredit,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&user).Update("points", gorm.Expr("points - ?", points)).Error; err != nil {
			return err
		}
		record := domain.PointRecord{
			UID: uid, Source: "activity_boost", Description: "活动积分插队", Points: -points,
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return ActivityResult{}, err
	}
	queueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if err := p.activity.AddPriority(queueCtx, activityType, uid, points); err != nil {
		slog.WarnContext(ctx, "add priority activity credit failed", "type", activityType, "error", err)
		_ = p.activity.InvalidateSeed(queueCtx, activityType)
	}
	p.broadcastActivityUpdate(ctx, activityType)
	return p.ActivityDetail(ctx, uid, activityType)
}

func (p *Platform) UseActivity(ctx context.Context, uid, activityType string) (ActivityUseResult, error) {
	if _, ok := domain.ValidActivityTypes[activityType]; !ok {
		return ActivityUseResult{}, domain.FieldError{Field: "type", Message: "unsupported activity type"}
	}
	var claimantCount int64
	if err := p.db.WithContext(ctx).Model(&domain.ActivityContent{}).
		Where("uid = ? AND type = ?", uid, activityType).Count(&claimantCount).Error; err != nil {
		return ActivityUseResult{}, fmt.Errorf("check activity claimant: %w", err)
	}
	if claimantCount == 0 {
		return ActivityUseResult{}, domain.ErrNotFound
	}
	if err := p.ensureActivityQueues(ctx, activityType); err != nil {
		return ActivityUseResult{}, err
	}

	// The claim count only rises on a served claim: a click that finds nothing
	// is a failed claim and leaves every counter untouched. The increment
	// happens inside the delivery transaction itself.
	var claimant domain.ActivityContent
	if err := p.db.WithContext(ctx).Where("uid = ? AND type = ?", uid, activityType).
		First(&claimant).Error; err != nil {
		return ActivityUseResult{}, fmt.Errorf("load activity claimant: %w", err)
	}
	// Publishers the claimant already received are skipped during selection —
	// each person's content serves a given claimant at most once.
	claimed, err := p.listActivityClaims(ctx, uid, activityType)
	if err != nil {
		return ActivityUseResult{}, err
	}

	if result, ok, err := p.claimPriorityActivity(ctx, claimant, activityType, claimed); err != nil {
		return ActivityUseResult{}, err
	} else if ok {
		return p.activityUseResultWithAvailability(ctx, uid, result)
	}
	result, err := p.claimCursorActivity(ctx, claimant, activityType, claimed)
	if err != nil {
		return ActivityUseResult{}, err
	}
	return p.activityUseResultWithAvailability(ctx, uid, result)
}

func (p *Platform) listActivityClaims(ctx context.Context, claimantUID, activityType string) ([]string, error) {
	var publishers []string
	if err := p.db.WithContext(ctx).Model(&domain.ActivityClaim{}).
		Where("claimant_uid = ? AND type = ?", claimantUID, activityType).
		Order("id ASC").Pluck("publisher_uid", &publishers).Error; err != nil {
		return nil, fmt.Errorf("list activity claims: %w", err)
	}
	return publishers, nil
}

// bumpActivityClaimCount charges one claim to the claimant inside the delivery
// transaction, so a claim that never serves content never counts.
func bumpActivityClaimCount(tx *gorm.DB, claimant *domain.ActivityContent) error {
	if err := tx.Model(&domain.ActivityContent{}).
		Where("uid = ? AND type = ?", claimant.UID, claimant.Type).
		Updates(map[string]any{
			"ordinary_credit": gorm.Expr("ordinary_credit + 1"),
			"used_count":      gorm.Expr("used_count + 1"),
		}).Error; err != nil {
		return err
	}
	claimant.ClaimCount++
	claimant.OrdinaryCredit++
	return nil
}

func (p *Platform) ensureActivityQueues(ctx context.Context, activityType string) error {
	seeded, err := p.activity.Seeded(ctx, activityType)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}
	if err := p.activity.Flush(ctx, activityType); err != nil {
		return err
	}
	var items []domain.ActivityContent
	if err := p.db.WithContext(ctx).Where("type = ?", activityType).Order("id ASC").Find(&items).Error; err != nil {
		return fmt.Errorf("load activity queue seed: %w", err)
	}
	for _, item := range items {
		if err := p.activity.EnqueueOrdinary(ctx, activityType, item.UID, item.OrdinaryCredit); err != nil {
			return err
		}
		if item.PriorityCredit > 0 {
			if err := p.activity.EnqueuePriority(ctx, activityType, item.UID, item.PriorityCredit); err != nil {
				return err
			}
		}
	}
	return p.activity.MarkSeeded(ctx, activityType)
}

func (p *Platform) claimPriorityActivity(ctx context.Context, claimant domain.ActivityContent, activityType string, claimed []string) (ActivityUseResult, bool, error) {
	excluded := append([]string(nil), claimed...)
	for attempt := 0; attempt < maxActivityQueueAttempts; attempt++ {
		candidateUID, err := p.activity.NextPriority(ctx, activityType, claimant.UID, excluded)
		if errors.Is(err, domain.ErrQueueEmpty) {
			return ActivityUseResult{}, false, nil
		}
		if err != nil {
			return ActivityUseResult{}, false, err
		}
		result, err := p.deliverPriorityActivity(ctx, claimant, candidateUID, activityType)
		if errors.Is(err, errActivityAlreadyClaimed) {
			excluded = append(excluded, candidateUID)
			continue
		}
		if errors.Is(err, errStaleActivityQueueEntry) {
			if removeErr := p.activity.RemovePriority(ctx, activityType, candidateUID); removeErr != nil {
				return ActivityUseResult{}, false, removeErr
			}
			continue
		}
		if err != nil {
			return ActivityUseResult{}, false, err
		}
		return result, true, nil
	}
	return ActivityUseResult{}, false, nil
}

// claimCursorActivity serves ordinary claims through the activity's FIFO
// cursor: one shared position that only successful claims advance, walking
// the queue in publish order. It skips the claimant themselves, publishers
// whose granted chances are used up (parked), and every publisher this
// claimant has already received — a publisher's content serves a given
// claimant at most once, ever, so once everyone has served them the claim
// reports ErrQueueEmpty. Publishers park when their chances run out and
// re-activate through the chance they earn with each of their own claim
// clicks.
func (p *Platform) claimCursorActivity(ctx context.Context, claimant domain.ActivityContent, activityType string, claimed []string) (ActivityUseResult, error) {
	for attempt := 0; attempt < maxActivityQueueAttempts; attempt++ {
		candidateUID, err := p.activity.NextByCursor(ctx, activityType, claimant.UID, claimed)
		if err != nil {
			return ActivityUseResult{}, err
		}
		result, err := p.deliverCursorActivity(ctx, claimant, candidateUID, activityType)
		if errors.Is(err, errStaleActivityQueueEntry) {
			if removeErr := p.activity.RemoveOrdinary(ctx, activityType, candidateUID); removeErr != nil {
				return ActivityUseResult{}, removeErr
			}
			continue
		}
		if err != nil {
			return ActivityUseResult{}, err
		}
		return result, nil
	}
	return ActivityUseResult{}, domain.ErrQueueEmpty
}

// deliverCursorActivity hands the cursor-selected publisher's content to the
// claimant. The claim record keeps one row per claimant-publisher pair — it
// drives the exclusion list that keeps served publishers from ever being
// picked again for that claimant — and serving costs the publisher one of
// their ordinary chances, so a first publish is served at most three times
// until more chances are earned.
func (p *Platform) deliverCursorActivity(
	ctx context.Context,
	claimant domain.ActivityContent,
	candidateUID, activityType string,
) (ActivityUseResult, error) {
	var candidate domain.ActivityContent
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("uid = ? AND type = ?", candidateUID, activityType).First(&candidate).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errStaleActivityQueueEntry
			}
			return err
		}
		if candidate.OrdinaryCredit <= 0 {
			return errStaleActivityQueueEntry
		}
		candidate.OrdinaryRounds++
		candidate.OrdinaryCredit--
		if err := tx.Model(&candidate).UpdateColumns(map[string]any{
			"ordinary_rounds": candidate.OrdinaryRounds,
			"ordinary_credit": candidate.OrdinaryCredit,
		}).Error; err != nil {
			return err
		}
		claimRecord := domain.ActivityClaim{ClaimantUID: claimant.UID, PublisherUID: candidate.UID, Type: activityType}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&claimRecord).Error; err != nil {
			return err
		}
		return bumpActivityClaimCount(tx, &claimant)
	})
	if err != nil {
		return ActivityUseResult{}, err
	}
	queueContext, cancelQueue := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancelQueue()
	if err := p.activity.AddOrdinary(queueContext, activityType, candidateUID, -1); err != nil {
		slog.WarnContext(ctx, "consume ordinary activity credit failed", "type", activityType, "error", err)
		_ = p.activity.InvalidateSeed(queueContext, activityType)
	}
	if err := p.activity.AddOrdinary(queueContext, activityType, claimant.UID, 1); err != nil {
		slog.WarnContext(ctx, "add earned activity credit failed", "type", activityType, "error", err)
		_ = p.activity.InvalidateSeed(queueContext, activityType)
	}
	notifyContext, cancelNotify := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancelNotify()
	if err := p.updates.Publish(notifyContext, candidateUID, activityType); err != nil {
		slog.WarnContext(ctx, "activity update notification failed", "type", activityType, "error", err)
	}
	if err := p.updates.PublishAll(notifyContext, activityType); err != nil {
		slog.WarnContext(ctx, "activity update broadcast failed", "type", activityType, "error", err)
	}
	return ActivityUseResult{
		Content: candidate.Content,
		Source:  "ordinary",
		State:   activityResult(claimant),
	}, nil
}

func (p *Platform) deliverPriorityActivity(
	ctx context.Context,
	claimant domain.ActivityContent,
	candidateUID, activityType string,
) (ActivityUseResult, error) {
	var candidate domain.ActivityContent
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("uid = ? AND type = ?", candidateUID, activityType).First(&candidate).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errStaleActivityQueueEntry
			}
			return err
		}
		if candidate.PriorityCredit <= 0 {
			return errStaleActivityQueueEntry
		}
		// Recording the pair inside the transaction makes "one serve per
		// claimant-publisher" atomic: a duplicate key rolls the credit change
		// back and the caller moves on to the next candidate.
		claimRecord := domain.ActivityClaim{ClaimantUID: claimant.UID, PublisherUID: candidate.UID, Type: activityType}
		claimResult := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&claimRecord)
		if claimResult.Error != nil {
			return claimResult.Error
		}
		if claimResult.RowsAffected == 0 {
			return errActivityAlreadyClaimed
		}
		candidate.PriorityCredit--
		candidate.PriorityRounds++
		if err := tx.Model(&candidate).UpdateColumns(map[string]any{
			"boost_rounds":    candidate.PriorityRounds,
			"priority_credit": candidate.PriorityCredit,
		}).Error; err != nil {
			return err
		}
		return bumpActivityClaimCount(tx, &claimant)
	})
	if err != nil {
		return ActivityUseResult{}, err
	}
	queueContext, cancelQueue := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancelQueue()
	if err := p.activity.AddPriority(queueContext, activityType, candidateUID, -1); err != nil {
		slog.WarnContext(ctx, "consume priority activity credit failed", "type", activityType, "error", err)
		_ = p.activity.InvalidateSeed(queueContext, activityType)
	}
	if err := p.activity.AddOrdinary(queueContext, activityType, claimant.UID, 1); err != nil {
		slog.WarnContext(ctx, "add earned activity credit failed", "type", activityType, "error", err)
		_ = p.activity.InvalidateSeed(queueContext, activityType)
	}
	notifyContext, cancelNotify := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancelNotify()
	if err := p.updates.Publish(notifyContext, candidateUID, activityType); err != nil {
		slog.WarnContext(ctx, "activity update notification failed", "type", activityType, "error", err)
	}
	if err := p.updates.PublishAll(notifyContext, activityType); err != nil {
		slog.WarnContext(ctx, "activity update broadcast failed", "type", activityType, "error", err)
	}
	return ActivityUseResult{
		Content: candidate.Content,
		Source:  "priority",
		State:   activityResult(claimant),
	}, nil
}

func (p *Platform) Points(ctx context.Context, uid string) (int64, error) {
	user, err := p.EnsureUser(ctx, uid)
	if err != nil {
		return 0, err
	}
	return user.Points, nil
}

func (p *Platform) PointsHistory(ctx context.Context, uid string, limit, offset int) ([]domain.PointRecord, error) {
	if _, err := p.EnsureUser(ctx, uid); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var records []domain.PointRecord
	if err := p.db.WithContext(ctx).Where("uid = ? AND points > 0", uid).
		Order("id DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list points history: %w", err)
	}
	return records, nil
}

func (p *Platform) Exchange(ctx context.Context, uid, code string) (ExchangeResult, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return ExchangeResult{}, domain.FieldError{Field: "code", Message: "is required"}
	}
	if _, err := p.EnsureUser(ctx, uid); err != nil {
		return ExchangeResult{}, err
	}
	var exchange domain.ExchangeCode
	var total int64
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("code = ?", code).First(&exchange).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if exchange.Status != domain.ExchangeStatusUnused {
			return domain.ErrAlreadyUsed
		}
		now := p.now()
		if err := tx.Model(&exchange).Updates(map[string]any{
			"status": domain.ExchangeStatusUsed, "used_uid": uid, "used_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&domain.User{}).Where("uid = ?", uid).
			Update("points", gorm.Expr("points + ?", exchange.Points)).Error; err != nil {
			return err
		}
		if err := tx.Create(&domain.PointRecord{
			UID: uid, Source: "exchange_code", Description: "兑换码奖励", Points: exchange.Points,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&domain.User{}).Select("points").Where("uid = ?", uid).Scan(&total).Error
	})
	if err != nil {
		return ExchangeResult{}, err
	}
	return ExchangeResult{AwardedPoints: exchange.Points, TotalPoints: total}, nil
}

func (p *Platform) Notice(ctx context.Context, noticeType string) (domain.Notice, error) {
	var notice domain.Notice
	err := p.db.WithContext(ctx).First(&notice, "type = ?", noticeType).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Notice{Type: noticeType}, nil
	}
	if err != nil {
		return domain.Notice{}, fmt.Errorf("load notice: %w", err)
	}
	return notice, nil
}

func (p *Platform) GroupQRCode(ctx context.Context) (string, error) {
	var setting domain.Setting
	err := p.db.WithContext(ctx).First(&setting, "`key` = ?", groupQRCodeKey).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load group qrcode: %w", err)
	}
	return setting.Value, nil
}

func validateDigits(value string, minLength, maxLength int) error {
	if len(value) < minLength || len(value) > maxLength {
		return domain.FieldError{Field: "code", Message: fmt.Sprintf("must contain %d or %d digits", minLength, maxLength)}
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return domain.FieldError{Field: "code", Message: "must contain digits only"}
		}
	}
	return nil
}

func maskCode(code string) string {
	if len(code) < 6 {
		return "***"
	}
	return code[:3] + "**" + code[len(code)-3:]
}

func maskIdentifier(value string) string {
	characters := []rune(strings.TrimSpace(value))
	if len(characters) == 0 {
		return "未知"
	}
	if len(characters) <= 2 {
		return string(characters[0]) + "***"
	}
	if len(characters) <= 6 {
		return string(characters[0]) + "***" + string(characters[len(characters)-1])
	}
	return string(characters[:3]) + "***" + string(characters[len(characters)-3:])
}

func activityResult(item domain.ActivityContent) ActivityResult {
	return ActivityResult{
		Type: item.Type, Content: item.Content, Published: true,
		OrdinaryRounds: item.OrdinaryRounds, OrdinaryCredit: item.OrdinaryCredit,
		PriorityRounds:  item.PriorityRounds,
		PointsCommitted: item.PointsCommitted, PriorityCredit: item.PriorityCredit,
		ClaimCount:  item.ClaimCount,
		PublishedAt: &item.CreatedAt, UpdatedAt: &item.UpdatedAt,
	}
}

func (p *Platform) activityResultWithAvailability(
	ctx context.Context, uid string, item domain.ActivityContent,
) (ActivityResult, error) {
	result := activityResult(item)
	canClaim, err := p.canClaimActivity(ctx, uid, item.Type)
	if err != nil {
		return ActivityResult{}, err
	}
	result.CanClaim = canClaim
	return result, nil
}

func (p *Platform) activityUseResultWithAvailability(
	ctx context.Context, uid string, result ActivityUseResult,
) (ActivityUseResult, error) {
	canClaim, err := p.canClaimActivity(ctx, uid, result.State.Type)
	if err != nil {
		return ActivityUseResult{}, err
	}
	result.State.CanClaim = canClaim
	return result, nil
}

// broadcastActivityUpdate fans a best-effort realtime event out to every
// client watching the activity type.
func (p *Platform) broadcastActivityUpdate(ctx context.Context, activityType string) {
	notifyContext, cancelNotify := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancelNotify()
	if err := p.updates.PublishAll(notifyContext, activityType); err != nil {
		slog.WarnContext(ctx, "activity update broadcast failed", "type", activityType, "error", err)
	}
}

// canClaimActivity reports whether a claim click could succeed right now:
// some other publisher still holds ordinary or priority chances this claimant
// has not received yet — a publisher's content serves a given claimant at
// most once, ever.
func (p *Platform) canClaimActivity(ctx context.Context, uid, activityType string) (bool, error) {
	var count int64
	received := p.db.WithContext(ctx).Model(&domain.ActivityClaim{}).
		Select("publisher_uid").
		Where("claimant_uid = ? AND type = ?", uid, activityType)
	err := p.db.WithContext(ctx).Model(&domain.ActivityContent{}).
		Where("type = ? AND uid <> ? AND (ordinary_credit > 0 OR priority_credit > 0)", activityType, uid).
		Where("uid NOT IN (?)", received).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check activity availability: %w", err)
	}
	return count > 0, nil
}
