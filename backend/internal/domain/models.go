package domain

import "time"

const (
	LuckyStatusAvailable = "available"
	LuckyStatusUsed      = "used"

	ExchangeStatusUnused = "unused"
	ExchangeStatusUsed   = "used"

	ActivityBuyFood       = "buy_food"
	ActivityCashTurntable = "cash_turntable"
	ActivityCashMonopoly  = "cash_monopoly"
	ActivityDailyCash     = "daily_cash"
)

var ValidActivityTypes = map[string]struct{}{
	ActivityBuyFood: {}, ActivityCashTurntable: {}, ActivityCashMonopoly: {}, ActivityDailyCash: {},
}

type User struct {
	ID           uint      `gorm:"primaryKey" json:"-"`
	UID          string    `gorm:"size:40;uniqueIndex;not null" json:"uid"`
	Phone        *string   `gorm:"size:20;uniqueIndex" json:"phone,omitempty"`
	InvitedByUID *string   `gorm:"size:40;index" json:"-"`
	Points       int64     `gorm:"not null;default:0" json:"points"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type LuckyCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UID       string     `gorm:"size:40;index;not null" json:"uid"`
	Code      string     `gorm:"size:9;uniqueIndex;not null" json:"code"`
	Status    string     `gorm:"size:20;index;not null;default:available" json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

type ActivityContent struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	UID             string    `gorm:"size:40;uniqueIndex:uid_activity_type;not null" json:"uid"`
	Type            string    `gorm:"size:30;uniqueIndex:uid_activity_type;not null" json:"type"`
	Content         string    `gorm:"type:varchar(200);not null" json:"content"`
	OrdinaryRounds  int64     `gorm:"not null;default:0" json:"ordinary_rounds"`
	OrdinaryCredit  int64     `gorm:"not null;default:0" json:"ordinary_credit"`
	PriorityRounds  int64     `gorm:"column:boost_rounds;not null;default:0" json:"priority_rounds"`
	PointsCommitted int64     `gorm:"column:boost_points_used;not null;default:0" json:"points_committed"`
	PriorityCredit  int64     `gorm:"not null;default:0" json:"priority_credit"`
	ClaimCount      int64     `gorm:"column:used_count;not null;default:0" json:"claim_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Notice struct {
	Type      string    `gorm:"size:50;primaryKey" json:"type"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Setting struct {
	Key       string    `gorm:"size:80;primaryKey" json:"key"`
	Value     string    `gorm:"type:text;not null" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ExchangeCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Code      string     `gorm:"size:40;uniqueIndex;not null" json:"code"`
	Points    int64      `gorm:"not null" json:"points"`
	Status    string     `gorm:"size:20;index;not null;default:unused" json:"status"`
	UsedUID   *string    `gorm:"size:40;index" json:"used_uid,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

type RechargeRecord struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Phone      string    `gorm:"size:20;index;not null" json:"phone"`
	Points     int64     `gorm:"not null" json:"points"`
	AdminPhone string    `gorm:"size:20;not null" json:"admin_phone"`
	CreatedAt  time.Time `json:"created_at"`
}

type PointRecord struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UID         string    `gorm:"size:40;index;not null" json:"uid"`
	Source      string    `gorm:"size:40;index;not null" json:"source"`
	Description string    `gorm:"size:200;not null" json:"description"`
	Points      int64     `gorm:"not null" json:"points"`
	CreatedAt   time.Time `json:"created_at"`
}

type Feedback struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UID       string    `gorm:"size:40;index;not null" json:"uid"`
	Content   string    `gorm:"type:varchar(500);not null" json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type ActivityStats struct {
	Type       string `json:"type"`
	Published  bool   `json:"published"`
	ClaimCount int64  `json:"claim_count"`
}
