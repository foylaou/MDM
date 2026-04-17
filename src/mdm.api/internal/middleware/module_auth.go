package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/port"
)

// AuthHelper provides unified auth + module permission checking for REST controllers.
type AuthHelper struct {
	jwtSecret      string
	permissionRepo port.PermissionRepository
}

func NewAuthHelper(jwtSecret string, permissionRepo port.PermissionRepository) *AuthHelper {
	return &AuthHelper{jwtSecret: jwtSecret, permissionRepo: permissionRepo}
}

// RequireAuth validates JWT and returns claims. No module permission check.
func (h *AuthHelper) RequireAuth(r *http.Request) (*Claims, error) {
	return ExtractTokenFromRequest(r, h.jwtSecret)
}

// RequireModule validates JWT and checks that the user has at least minLevel
// permission on the given module. sys_admin bypasses all module checks.
func (h *AuthHelper) RequireModule(r *http.Request, module string, minLevel string) (*Claims, error) {
	claims, err := h.RequireAuth(r)
	if err != nil {
		return nil, err
	}
	// sys_admin bypasses module checks
	if claims.SystemRole == "sys_admin" {
		return claims, nil
	}
	// Legacy admin role also bypasses (backward compat during migration)
	if claims.Role == "admin" {
		return claims, nil
	}
	// Check module permission
	perm, err := h.permissionRepo.GetByUserAndModule(r.Context(), claims.UserID, module)
	if err != nil {
		return nil, fmt.Errorf("no permission for module %s", module)
	}
	if !IsLevelSufficient(perm.Permission, minLevel) {
		return nil, fmt.Errorf("insufficient permission: need %s, have %s", minLevel, perm.Permission)
	}
	return claims, nil
}

// RequireSysAdmin validates JWT and checks sys_admin or legacy admin role.
func (h *AuthHelper) RequireSysAdmin(r *http.Request) (*Claims, error) {
	claims, err := h.RequireAuth(r)
	if err != nil {
		return nil, err
	}
	if claims.SystemRole == "sys_admin" || claims.Role == "admin" {
		return claims, nil
	}
	return nil, fmt.Errorf("sys_admin required")
}

// GetUserPermissions returns all module permissions for a user.
func (h *AuthHelper) GetUserPermissions(ctx context.Context, userID string) ([]*domain.ModulePermission, error) {
	return h.permissionRepo.GetByUserID(ctx, userID)
}

// Permission level hierarchy
var levelOrder = map[string]int{
	"none":      0,
	"viewer":    1,
	"requester": 2,
	"operator":  3,
	"approver":  4,
	"manager":   5,
}

// IsLevelSufficient checks if actual permission level meets the required minimum.
func IsLevelSufficient(actual, required string) bool {
	return levelOrder[actual] >= levelOrder[required]
}
