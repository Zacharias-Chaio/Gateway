package gatewayruntime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"gateway/internal/config"
	"gateway/internal/store"
)

// prepareDB 建立内存库并预写一份「不落文件、不输出终端」的设置，避免测试写日志文件。
func prepareDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	settings := config.DefaultSettings()
	settings.App.Log.Console = false
	settings.App.Log.File = ""
	if err := config.SaveSettings(db, settings); err != nil {
		t.Fatalf("写入默认设置失败: %v", err)
	}
	model := store.DeviceModel{
		ID: "m1", ProfileIndex: 0, Name: "电表模型",
		Profile:    datatypes.JSON(`{"protocolType":"Modbus TCP"}`),
		Properties: datatypes.JSON(`[{"id":"v","name":"电压","dataType":"int","readFunctionCode":3,"registerBase":0,"registerOffset":0,"startBit":0,"endBit":15,"accessMode":"r"}]`),
	}
	if err := db.Create(&model).Error; err != nil {
		t.Fatalf("创建设备模型失败: %v", err)
	}
	return db
}

func createChannel(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	ch := store.Channel{
		Name:    name,
		Type:    "Network",
		Config:  datatypes.JSON(`{"deviceIp":"127.0.0.1","devicePort":1,"pollInterval":60000}`),
		Devices: datatypes.JSON(`[{"index":0,"commNo":1,"name":"","modelId":"m1"}]`),
	}
	if err := db.Create(&ch).Error; err != nil {
		t.Fatalf("创建链路失败: %v", err)
	}
}

// statusLen 序列化引擎状态快照以避免依赖未导出类型。
func statusLen(t *testing.T, m *Manager) int {
	t.Helper()
	raw, err := json.Marshal(m.Status())
	if err != nil {
		t.Fatalf("序列化引擎状态失败: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("解析引擎状态失败: %v", err)
	}
	return len(rows)
}

func TestConfigChangedReappliesPlans(t *testing.T) {
	db := prepareDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m, err := New(ctx, db)
	if err != nil {
		t.Fatalf("启动运行时失败: %v", err)
	}
	defer m.Stop()

	if n := statusLen(t, m); n != 0 {
		t.Fatalf("初始状态应为 0 条链路: %d", n)
	}

	// 保存一条链路后通知配置变更：引擎应加载对应 worker。
	createChannel(t, db, "链路A")
	m.ConfigChanged()
	deadline := time.Now().Add(2 * time.Second)
	for statusLen(t, m) != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := statusLen(t, m); n != 1 {
		t.Fatalf("ConfigChanged 后应加载 1 条链路: %d", n)
	}

	// 再保存一条并通知：差量热重载后应为 2 条。
	createChannel(t, db, "链路B")
	m.ConfigChanged()
	deadline = time.Now().Add(2 * time.Second)
	for statusLen(t, m) != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := statusLen(t, m); n != 2 {
		t.Fatalf("第二次 ConfigChanged 后应加载 2 条链路: %d", n)
	}
}

func TestDbPlanSourceLoadsSpecs(t *testing.T) {
	db := prepareDB(t)
	createChannel(t, db, "链路A")
	src := &dbPlanSource{db: db}

	channels, err := src.LoadChannels(context.Background())
	if err != nil || len(channels) != 1 {
		t.Fatalf("LoadChannels 失败: channels=%d err=%v", len(channels), err)
	}
	if channels[0].Type != "Network" || channels[0].ID == 0 || len(channels[0].Devices) == 0 {
		t.Fatalf("ChannelSpec 字段错误: %+v", channels[0])
	}

	models, err := src.LoadModels(context.Background())
	if err != nil || len(models) != 1 {
		t.Fatalf("LoadModels 失败: models=%d err=%v", len(models), err)
	}
	if models[0].ID != "m1" || len(models[0].Properties) == 0 {
		t.Fatalf("ModelSpec 字段错误: %+v", models[0])
	}
}
