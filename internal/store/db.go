package store

import (
	"os"
	"path/filepath"

	"gateway/internal/logx"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open 打开（或创建）SQLite 配置库并自动迁移表结构。
func Open(path string) (*gorm.DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logx.NewGormLogger(gormlogger.Warn),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&DeviceModel{}, &Channel{}); err != nil {
		return nil, err
	}
	return db, nil
}
