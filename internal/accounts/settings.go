package accounts

import (
	"go.uber.org/zap"
	"gorm.io/gorm"

	"formlander/internal/pkg/dbtxn"
)

// SetupDefaultSettings initializes default settings in the database
func SetupDefaultSettings(db *gorm.DB) error {
	// No default settings needed at this time
	return nil
}

// GetSetting retrieves a setting value by key
func GetSetting(db *gorm.DB, key string) (string, error) {
	var setting Settings
	if err := db.Where("key = ?", key).First(&setting).Error; err != nil {
		return "", err
	}
	return setting.Value, nil
}

// SetSetting updates or creates a setting
func SetSetting(db *gorm.DB, logger *zap.Logger, key, value string) error {
	var setting Settings
	err := db.Where("key = ?", key).First(&setting).Error

	if err == gorm.ErrRecordNotFound {
		// Create new setting
		setting = Settings{
			Key:   key,
			Value: value,
		}
		return dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
			return tx.Create(&setting).Error
		})
	} else if err != nil {
		return err
	}

	// Update existing setting
	setting.Value = value
	return dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Save(&setting).Error
	})
}
