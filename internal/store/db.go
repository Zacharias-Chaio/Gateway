package store

import (
	"os"
	"path/filepath"

	"gateway/internal/config"
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
	if err := db.AutoMigrate(&DeviceModel{}, &Channel{}, &config.Record{}); err != nil {
		return nil, err
	}
	return db, nil
}

// LegacyChannelCAN 是历史版本支持的 CAN 链路类型。采集业务已收窄为
// Modbus 串口 / 网络，启动时物理删除该类型的历史链路记录。
const LegacyChannelCAN = "CAN"

// DeleteUnsupportedChannels 物理删除采集引擎不再支持的链路记录（当前为 CAN），
// 返回删除的行数。属于一次性破坏性迁移，供 main 启动时调用。
func DeleteUnsupportedChannels(db *gorm.DB) (int64, error) {
	res := db.Where("type = ?", LegacyChannelCAN).Delete(&Channel{})
	return res.RowsAffected, res.Error
}
