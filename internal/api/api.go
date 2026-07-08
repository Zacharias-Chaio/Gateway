package api

import (
	"encoding/json"
	"net/http"

	"gateway/internal/store"

	"gorm.io/gorm"
)

// Server 持有数据库句柄，挂载所有 REST 接口。
type Server struct {
	DB *gorm.DB
	// HardwarePath 指向描述硬件接口的 YAML 配置文件。
	HardwarePath string
	// Engine 负责链路的运行与热重载；链路配置变更后回调其 Apply。
	// 允许为 nil（例如未启用引擎的场景）。
	Engine Reloader
}

// Reloader 抽象引擎的热重载能力，供链路增删改后触发。
// 由 engine.Engine 实现，接口化以避免 api 直接依赖引擎内部细节。
type Reloader interface {
	// Apply 以给定链路集合为期望状态做差量启停。
	Apply(channels []store.Channel)
}

func New(db *gorm.DB, hardwarePath string) *Server {
	if hardwarePath == "" {
		hardwarePath = "hardware.yaml"
	}
	return &Server{DB: db, HardwarePath: hardwarePath}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": data})
}
func fail(w http.ResponseWriter, c int, m string) {
	writeJSON(w, c, map[string]any{"code": c, "msg": m})
}
