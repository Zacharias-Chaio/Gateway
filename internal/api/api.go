package api

import (
	"encoding/json"
	"net/http"

	"gorm.io/gorm"
)

// Server 持有数据库句柄，挂载所有 REST 接口。
type Server struct {
	DB *gorm.DB
	// HardwarePath 指向描述硬件接口的 YAML 配置文件。
	HardwarePath string
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
