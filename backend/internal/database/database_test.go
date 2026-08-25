package database

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrateRestoresOrdinaryRoundsFromPreviousCreditMigrationOnce(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.ActivityContent{}, &domain.Setting{}))
	require.NoError(t, db.Create(&[]domain.ActivityContent{
		{UID: "positive", Type: domain.ActivityBuyFood, Content: "a", ClaimCount: 7, OrdinaryRounds: 1},
		{UID: "zero", Type: domain.ActivityBuyFood, Content: "b", ClaimCount: 5, OrdinaryRounds: 0},
	}).Error)
	require.NoError(t, db.Create(&domain.Setting{Key: activityOrdinaryCreditMigrationKey, Value: "complete"}).Error)

	require.NoError(t, Migrate(db))
	var positive domain.ActivityContent
	require.NoError(t, db.Where("uid = ?", "positive").First(&positive).Error)
	require.Equal(t, int64(6), positive.OrdinaryRounds)
	require.Equal(t, int64(1), positive.OrdinaryCredit)
	var zero domain.ActivityContent
	require.NoError(t, db.Where("uid = ?", "zero").First(&zero).Error)
	require.Equal(t, int64(5), zero.OrdinaryRounds)
	require.Zero(t, zero.OrdinaryCredit)

	require.NoError(t, db.Model(&positive).Update("ordinary_rounds", 4).Error)
	require.NoError(t, Migrate(db))
	require.NoError(t, db.Where("uid = ?", "positive").First(&positive).Error)
	require.Equal(t, int64(4), positive.OrdinaryRounds)
	require.Equal(t, int64(1), positive.OrdinaryCredit)
}

func TestMigratePreservesExistingOrdinaryRoundsWithoutPreviousCreditMigration(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.ActivityContent{}, &domain.Setting{}))
	require.NoError(t, db.Create(&[]domain.ActivityContent{
		{UID: "balanced", Type: domain.ActivityBuyFood, Content: "a", ClaimCount: 7, OrdinaryRounds: 6},
		{UID: "popular", Type: domain.ActivityBuyFood, Content: "b", ClaimCount: 5, OrdinaryRounds: 9},
	}).Error)

	require.NoError(t, Migrate(db))
	var balanced domain.ActivityContent
	require.NoError(t, db.Where("uid = ?", "balanced").First(&balanced).Error)
	require.Equal(t, int64(6), balanced.OrdinaryRounds)
	require.Equal(t, int64(1), balanced.OrdinaryCredit)
	var popular domain.ActivityContent
	require.NoError(t, db.Where("uid = ?", "popular").First(&popular).Error)
	require.Equal(t, int64(5), popular.OrdinaryRounds)
	require.Zero(t, popular.OrdinaryCredit)
}

func TestMigrateDerivesCreditWhenPreviousRoundRestorationAlreadyRan(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.ActivityContent{}, &domain.Setting{}))
	require.NoError(t, db.Create(&domain.ActivityContent{
		UID: "restored", Type: domain.ActivityBuyFood, Content: "a", ClaimCount: 15, OrdinaryRounds: 7,
	}).Error)
	require.NoError(t, db.Create(&[]domain.Setting{
		{Key: activityOrdinaryCreditMigrationKey, Value: "complete"},
		{Key: activityOrdinaryRoundMigrationKey, Value: "complete"},
	}).Error)

	require.NoError(t, Migrate(db))
	var restored domain.ActivityContent
	require.NoError(t, db.Where("uid = ?", "restored").First(&restored).Error)
	require.Equal(t, int64(7), restored.OrdinaryRounds)
	require.Equal(t, int64(8), restored.OrdinaryCredit)
}

func TestMigrateClampsRoundsAndRebuildsCreditFromClaimCount(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.ActivityContent{}, &domain.Setting{}))
	require.NoError(t, db.Create(&[]domain.ActivityContent{
		{UID: "over", Type: domain.ActivityBuyFood, Content: "a", ClaimCount: 14, OrdinaryRounds: 16, OrdinaryCredit: 7},
		{UID: "available", Type: domain.ActivityBuyFood, Content: "b", ClaimCount: 14, OrdinaryRounds: 8, OrdinaryCredit: 99},
	}).Error)
	require.NoError(t, db.Create(&domain.Setting{Key: activityRoundCreditMigrationKey, Value: "complete"}).Error)

	require.NoError(t, Migrate(db))
	var over domain.ActivityContent
	require.NoError(t, db.Where("uid = ?", "over").First(&over).Error)
	require.Equal(t, int64(14), over.OrdinaryRounds)
	require.Zero(t, over.OrdinaryCredit)
	var available domain.ActivityContent
	require.NoError(t, db.Where("uid = ?", "available").First(&available).Error)
	require.Equal(t, int64(8), available.OrdinaryRounds)
	require.Equal(t, int64(6), available.OrdinaryCredit)
}
