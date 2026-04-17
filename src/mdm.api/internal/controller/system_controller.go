package controller

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/config"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/service"
)

type SystemController struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewSystemController(pool *pgxpool.Pool, cfg *config.Config) *SystemController {
	return &SystemController{pool: pool, cfg: cfg}
}

func (c *SystemController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", c.handleHealth)
	mux.HandleFunc("/api/system-status", c.handleSystemStatus)
	mux.HandleFunc("/api/setup", c.handleSetup)
	mux.HandleFunc("/api/register", c.handleRegister)
	mux.HandleFunc("/api/ws-config", c.handleWSConfig)
}

// handleHealth godoc
// @Summary 健康檢查
// @Tags System
// @Success 200 {string} string "ok"
// @Router /health [get]
func (c *SystemController) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleSystemStatus godoc
// @Summary 系統初始化狀態
// @Tags System
// @Produce json
// @Success 200 {object} swagSystemStatus
// @Failure 500 {object} swagError
// @Router /api/system-status [get]
func (c *SystemController) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	var count int
	err := c.pool.QueryRow(r.Context(), "SELECT count(*) FROM users").Scan(&count)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if count == 0 {
		w.Write([]byte(`{"initialized":false}`))
	} else {
		w.Write([]byte(`{"initialized":true}`))
	}
}

// handleSetup godoc
// @Summary 初始化系統（建立第一位管理員）
// @Tags System
// @Accept json
// @Produce json
// @Param body body swagSetupReq true "管理員帳號資訊"
// @Success 200 {object} swagOK
// @Failure 400 {object} swagError
// @Failure 403 {object} swagError "系統已初始化"
// @Router /api/setup [post]
func (c *SystemController) handleSetup(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var count int
	if err := c.pool.QueryRow(r.Context(), "SELECT count(*) FROM users").Scan(&count); err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if count > 0 {
		writeError(w, http.StatusForbidden, "system already initialized")
		return
	}
	var body struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	hash, err := service.HashArgon2id(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	displayName := body.DisplayName
	if displayName == "" {
		displayName = body.Username
	}
	_, err = c.pool.Exec(r.Context(),
		`INSERT INTO users (username, password_hash, role, display_name) VALUES ($1, $2, 'admin', $3)`,
		body.Username, hash, displayName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	writeOK(w)
}

// handleRegister godoc
// @Summary 使用者自行註冊（需等待管理員啟用）
// @Tags System
// @Accept json
// @Produce json
// @Param body body swagRegisterReq true "註冊資訊"
// @Success 200 {object} swagOK
// @Failure 400 {object} swagError
// @Failure 409 {object} swagError "帳號已存在"
// @Router /api/register [post]
func (c *SystemController) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	if len(body.Password) < 6 {
		writeError(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}
	hash, err := service.HashArgon2id(body.Password)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	displayName := body.DisplayName
	if displayName == "" {
		displayName = body.Username
	}
	_, err = c.pool.Exec(r.Context(),
		`INSERT INTO users (username, password_hash, role, display_name, is_active) VALUES ($1, $2, 'viewer', $3, false)`,
		body.Username, hash, displayName)
	if err != nil {
		writeError(w, http.StatusConflict, "username already exists")
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "message": "registration successful, please wait for admin activation"})
}

// handleWSConfig godoc
// @Summary WebSocket 設定
// @Tags System
// @Produce json
// @Security BearerAuth
// @Success 200 {object} swagWSConfig
// @Failure 401 {string} string "Unauthorized"
// @Router /api/ws-config [get]
func (c *SystemController) handleWSConfig(w http.ResponseWriter, r *http.Request) {
	if _, err := middleware.ExtractTokenFromRequest(r, c.cfg.JWTSecret); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]interface{}{
		"backend_relay": c.cfg.WebSocketURL != "",
	})
}
