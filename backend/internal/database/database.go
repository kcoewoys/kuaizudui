package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/eaok-cn/kuaizudui/backend/internal/config"
	"github.com/eaok-cn/kuaizudui/backend/internal/domain"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	activityOrdinaryCreditMigrationKey = "migration.activity_ordinary_credit_v1"
	activityOrdinaryRoundMigrationKey  = "migration.activity_ordinary_round_v2"
	activityRoundCreditMigrationKey    = "migration.activity_round_credit_v3"
	activityQueueBalanceMigrationKey   = "migration.activity_queue_balance_v4"
)

func OpenMySQL(ctx context.Context, cfg config.MySQLConfig, debug bool) (*gorm.DB, error) {
	logMode := logger.Warn
	if debug {
		logMode = logger.Info
	}
	db, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{Logger: logger.Default.LogMode(logMode)})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get mysql connection pool: %w", err)
	}
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConnections)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConnections)
	sqlDB.SetConnMaxLifetime(cfg.ConnectionMaxLifetime.Value())
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return db, nil
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&domain.User{},
		&domain.LuckyCode{},
		&domain.ActivityContent{},
		&domain.ActivityClaim{},
		&domain.Notice{},
		&domain.Setting{},
		&domain.ExchangeCode{},
		&domain.RechargeRecord{},
		&domain.PointRecord{},
		&domain.Feedback{},
	); err != nil {
		return fmt.Errorf("auto migrate database: %w", err)
	}
	if err := migrateActivityRoundCredit(db); err != nil {
		return fmt.Errorf("migrate activity round and credit: %w", err)
	}
	if err := migrateActivityQueueBalance(db); err != nil {
		return fmt.Errorf("migrate activity queue balance: %w", err)
	}
	return nil
}

func migrateActivityRoundCredit(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var marker domain.Setting
		err := tx.Where(map[string]any{"key": activityRoundCreditMigrationKey}).First(&marker).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		creditMigrated, err := settingExists(tx, activityOrdinaryCreditMigrationKey)
		if err != nil {
			return err
		}
		roundsRestored, err := settingExists(tx, activityOrdinaryRoundMigrationKey)
		if err != nil {
			return err
		}

		var items []domain.ActivityContent
		if err := tx.Find(&items).Error; err != nil {
			return err
		}
		for index := range items {
			rounds := items[index].OrdinaryRounds
			credit := items[index].ClaimCount - rounds
			if creditMigrated && !roundsRestored {
				credit = items[index].OrdinaryRounds
				rounds = items[index].ClaimCount - credit
			}
			if credit < 0 {
				credit = 0
			}
			if rounds < 0 {
				rounds = 0
			}
			if err := tx.Model(&items[index]).UpdateColumns(map[string]any{
				"ordinary_rounds": rounds,
				"ordinary_credit": credit,
			}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&domain.Setting{Key: activityRoundCreditMigrationKey, Value: "complete"}).Error
	})
}

func settingExists(tx *gorm.DB, key string) (bool, error) {
	var count int64
	if err := tx.Model(&domain.Setting{}).Where("`key` = ?", key).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func migrateActivityQueueBalance(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		exists, err := settingExists(tx, activityQueueBalanceMigrationKey)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}

		var items []domain.ActivityContent
		if err := tx.Find(&items).Error; err != nil {
			return err
		}
		for index := range items {
			rounds := items[index].OrdinaryRounds
			if rounds > items[index].ClaimCount {
				rounds = items[index].ClaimCount
			}
			if rounds < 0 {
				rounds = 0
			}
			credit := items[index].ClaimCount - rounds
			if err := tx.Model(&items[index]).UpdateColumns(map[string]any{
				"ordinary_rounds": rounds,
				"ordinary_credit": credit,
			}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&domain.Setting{Key: activityQueueBalanceMigrationKey, Value: "complete"}).Error
	})
}
