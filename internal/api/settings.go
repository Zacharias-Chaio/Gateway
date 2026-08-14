package api

import (
	"encoding/json"
	"net/http"

	"gateway/internal/config"
)

// GetSettings returns all persisted application and hardware settings.
func (s *Server) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := config.LoadSettings(s.DB)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取网关设置失败: "+err.Error())
		return
	}
	ok(w, settings)
}

// SaveSettings persists all settings categories. Logging options apply immediately;
// gateway identity and NATS connection options take effect after a service restart.
func (s *Server) SaveSettings(w http.ResponseWriter, r *http.Request) {
	var settings config.Settings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		fail(w, http.StatusBadRequest, "JSON 解析失败: "+err.Error())
		return
	}
	if err := config.SaveSettings(s.DB, settings); err != nil {
		fail(w, http.StatusBadRequest, "保存网关设置失败: "+err.Error())
		return
	}
	applyLogSettings(settings.App)
	ok(w, settings)
}
