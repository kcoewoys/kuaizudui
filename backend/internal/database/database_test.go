package database

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSplitDatabaseDSNStripsDatabaseName(t *testing.T) {
	serverDSN, dbName, err := splitDatabaseDSN(
		"root:root@tcp(127.0.0.1:3306)/kuaizudui?charset=utf8mb4&parseTime=True&loc=Local")
	require.NoError(t, err)
	require.Equal(t, "kuaizudui", dbName)

	parsed, err := mysqldriver.ParseDSN(serverDSN)
	require.NoError(t, err)
	require.Empty(t, parsed.DBName)
	require.Equal(t, "tcp", parsed.Net)
	require.Equal(t, "127.0.0.1:3306", parsed.Addr)
	require.True(t, parsed.ParseTime)
}

func TestSplitDatabaseDSNAllowsMissingDatabase(t *testing.T) {
	serverDSN, dbName, err := splitDatabaseDSN("root:root@tcp(127.0.0.1:3306)/")
	require.NoError(t, err)
	require.Empty(t, dbName)
	require.NotEmpty(t, serverDSN)
}

func TestSplitDatabaseDSNRejectsInvalidDSN(t *testing.T) {
	_, _, err := splitDatabaseDSN("invalid")
	require.Error(t, err)
}

func TestCreateDatabaseStatementEscapesBackticks(t *testing.T) {
	require.Equal(t,
		"CREATE DATABASE IF NOT EXISTS `kuaizudui` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		createDatabaseStatement("kuaizudui"))
	require.Equal(t,
		"CREATE DATABASE IF NOT EXISTS `we``ird` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		createDatabaseStatement("we`ird"))
}

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
