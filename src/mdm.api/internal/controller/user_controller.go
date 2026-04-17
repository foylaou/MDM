package controller

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/anthropics/mdm-server/internal/adapter/postgres"
	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/service"
)

type UserController struct {
	userRepo       *postgres.UserRepo
	permissionRepo *postgres.PermissionRepo
	auth           *middleware.AuthHelper
}

func NewUserController(userRepo *postgres.UserRepo, permissionRepo *postgres.PermissionRepo, auth *middleware.AuthHelper) *UserController {
	return &UserController{userRepo: userRepo, permissionRepo: permissionRepo, auth: auth}
}

func (c *UserController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/users-permissions/", c.handleUserPermissions)
	mux.HandleFunc("/api/users/", c.handleUserByID)
	mux.HandleFunc("/api/users-list", c.handleUsersList)
}

// handleUserByID godoc
// @Summary 更新 / 刪除使用者（需 sys_admin）
// @Tags User
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "使用者 ID"
// @Param body body swagUserUpdateReq false "更新欄位（PUT）"
// @Success 200 {object} swagOK
// @Failure 401 {string} string "Unauthorized"
// @Router /api/users/{id} [put]
// @Router /api/users/{id} [delete]
func (c *UserController) handleUserByID(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireSysAdmin(r); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPut {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fields := map[string]interface{}{}
		for _, k := range []string{"role", "display_name", "is_active"} {
			if v, ok := body[k]; ok {
				fields[k] = v
			}
		}
		if pw, ok := body["password"].(string); ok && pw != "" {
			hash, err := service.HashArgon2id(pw)
			if err == nil {
				fields["password_hash"] = hash
			}
		}
		if len(fields) == 0 {
			writeOK(w)
			return
		}
		if err := c.userRepo.UpdateFields(r.Context(), id, fields); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeOK(w)
		return
	}

	if r.Method == http.MethodDelete {
		c.userRepo.Delete(r.Context(), id)
		writeOK(w)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

// handleUserPermissions godoc
// @Summary 取得 / 設定使用者模組權限（需 sys_admin）
// @Tags User
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "使用者 ID"
// @Success 200 {object} map[string]string "GET: {asset: 'operator', mdm: 'viewer', ...}"
// @Router /api/users-permissions/{id} [get]
// @Router /api/users-permissions/{id} [put]
func (c *UserController) handleUserPermissions(w http.ResponseWriter, r *http.Request) {
	claims, err := c.auth.RequireSysAdmin(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	userID := strings.TrimPrefix(r.URL.Path, "/api/users-permissions/")
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		perms, err := c.permissionRepo.GetByUserID(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		permMap := map[string]string{}
		for _, p := range perms {
			permMap[p.Module] = p.Permission
		}
		writeJSON(w, permMap)

	case http.MethodPut:
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		validModules := map[string]bool{"asset": true, "mdm": true, "rental": true}
		validLevels := map[string]bool{"none": true, "viewer": true, "requester": true, "operator": true, "approver": true, "manager": true}
		for module, level := range body {
			if !validModules[module] || !validLevels[level] {
				continue
			}
			if level == "none" {
				c.permissionRepo.Delete(r.Context(), userID, module)
			} else {
				grantedBy := claims.UserID
				c.permissionRepo.Set(r.Context(), &domain.ModulePermission{
					UserID: userID, Module: module, Permission: level, GrantedBy: &grantedBy,
				})
			}
		}
		writeOK(w)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleUsersList godoc
// @Summary 使用者列表
// @Tags User
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string][]swagUserListItem "{users: [...]}"
// @Router /api/users-list [get]
func (c *UserController) handleUsersList(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireAuth(r); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	users, err := c.userRepo.List(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	type u struct {
		ID          string `json:"id"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
		IsActive    bool   `json:"is_active"`
	}
	rows := make([]u, 0, len(users))
	for _, user := range users {
		rows = append(rows, u{
			ID: user.ID, Username: user.Username,
			DisplayName: user.DisplayName, Role: user.Role, IsActive: user.IsActive,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"users": rows})
}
