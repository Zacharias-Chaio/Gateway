// Package gatewayruntime manages restartable gateway runtime resources.
package gatewayruntime

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"gateway/internal/config"
	"gateway/internal/engine"
	"gateway/internal/logx"
	"gateway/internal/natsclient"
	"gateway/internal/store"

	"gorm.io/gorm"
)

// Manager owns the restartable engine and NATS client while the HTTP process remains running.
type Manager struct {
	ctx context.Context
	db  *gorm.DB
	log *slog.Logger

	mu     sync.RWMutex
	engine *engine.Engine
	nats   *natsclient.Client
	cancel context.CancelFunc

	restartMu  sync.Mutex
	restarting bool
}

// New starts the first runtime instance.
func New(ctx context.Context, db *gorm.DB) (*Manager, error) {
	m := &Manager{ctx: ctx, db: db, log: logx.Module("runtime")}
	if err := m.start(); err != nil {
		return nil, err
	}
	return m, nil
}

// Restart schedules an in-process restart and returns false while one is already underway.
func (m *Manager) Restart() bool {
	m.restartMu.Lock()
	defer m.restartMu.Unlock()
	if m.restarting {
		return false
	}
	m.restarting = true
	go func() {
		defer func() {
			m.restartMu.Lock()
			m.restarting = false
			m.restartMu.Unlock()
		}()
		m.stop()
		if err := m.start(); err != nil {
			m.log.Error("网关运行时重启失败", "err", err)
			return
		}
		m.log.Info("网关运行时重启完成")
	}()
	return true
}

// Stop releases runtime resources during process shutdown.
func (m *Manager) Stop() { m.stop() }

func (m *Manager) start() error {
	settings, err := config.EnsureSettings(m.db)
	if err != nil {
		return fmt.Errorf("初始化数据库配置: %w", err)
	}
	logx.Init(settings.App.LogOptions())
	m.log = logx.Module("runtime")

	runCtx, cancel := context.WithCancel(m.ctx)
	eng := engine.New(runCtx)
	var nats *natsclient.Client
	if settings.App.NATS.Enabled {
		nats, err = natsclient.New(runCtx, settings.App.Gateway.GWID, settings.App.NATS, m.db, eng)
		if err != nil {
			m.log.Warn("启动 NATS 客户端失败，数据扇出功能已禁用", "err", err)
		} else {
			eng.SetEventSink(nats)
		}
	}
	var channels []store.Channel
	if err := m.db.Order("id asc").Find(&channels).Error; err != nil {
		m.log.Warn("加载链路配置失败，引擎以空配置启动", "err", err)
	}
	var models []store.DeviceModel
	if err := m.db.Order("profile_index asc").Find(&models).Error; err != nil {
		m.log.Warn("加载设备模型失败，引擎以空模型启动", "err", err)
	}
	plans, warnings := engine.BuildPlans(channels, models)
	for _, warning := range warnings {
		m.log.Warn("采集计划构建警告", "warn", warning)
	}
	eng.Apply(plans, models)

	m.mu.Lock()
	m.engine, m.nats, m.cancel = eng, nats, cancel
	m.mu.Unlock()
	return nil
}

func (m *Manager) stop() {
	m.mu.Lock()
	eng, nats, cancel := m.engine, m.nats, m.cancel
	m.engine, m.nats, m.cancel = nil, nil, nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if eng != nil {
		eng.Stop()
	}
	if nats != nil {
		nats.Close()
	}
}

func (m *Manager) current() *engine.Engine {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.engine
}

// Apply delegates configuration changes to the active engine.
func (m *Manager) Apply(plans []engine.ChannelPlan, models []store.DeviceModel) {
	if eng := m.current(); eng != nil {
		eng.Apply(plans, models)
	}
}

// Submit delegates a command to the active engine.
func (m *Manager) Submit(channelID int, command engine.WriteCommand) bool {
	eng := m.current()
	return eng != nil && eng.Submit(channelID, command)
}

// Values returns a real-time value snapshot from the active engine.
func (m *Manager) Values(channelID int) map[string]engine.SessionEntry {
	if eng := m.current(); eng != nil {
		return eng.Values(channelID)
	}
	return map[string]engine.SessionEntry{}
}

// CommunicationSnapshot returns communication data from the active engine.
func (m *Manager) CommunicationSnapshot(channelID, deviceIndex int, afterSeq uint64, limit int) (engine.CommunicationSnapshot, bool) {
	if eng := m.current(); eng != nil {
		return eng.CommunicationSnapshot(channelID, deviceIndex, afterSeq, limit)
	}
	return engine.CommunicationSnapshot{}, false
}

// Status returns the active engine status for the HTTP status endpoint.
func (m *Manager) Status() any {
	if eng := m.current(); eng != nil {
		return eng.Status()
	}
	return []any{}
}
