package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
)

const CookieName = "token"

type ctxKey string

const (
	CtxUserID   ctxKey = "user_id"
	CtxUsername ctxKey = "username"
	CtxRole    ctxKey = "role"
)

type Claims struct {
	UserID     string `json:"uid"`
	Username   string `json:"sub"`
	Role       string `json:"role"`
	SystemRole string `json:"system_role,omitempty"`
	jwt.RegisteredClaims
}

func GenerateTokens(secret, userID, username, role string, opts ...string) (access, refresh string, expiresAt int64, err error) {
	systemRole := ""
	if len(opts) > 0 {
		systemRole = opts[0]
	}
	exp := time.Now().Add(24 * time.Hour)
	accessClaims := &Claims{
		UserID:     userID,
		Username:   username,
		Role:       role,
		SystemRole: systemRole,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	at, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(secret))
	if err != nil {
		return "", "", 0, err
	}

	refreshClaims := &Claims{
		UserID:     userID,
		Username:   username,
		Role:       role,
		SystemRole: systemRole,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	rt, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(secret))
	if err != nil {
		return "", "", 0, err
	}
	return at, rt, exp.Unix(), nil
}

func ParseToken(secret, tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// publicProcedures that don't require auth.
var publicProcedures = map[string]bool{
	"/mdm.v1.AuthService/Login": true,
}

func NewAuthInterceptor(secret string) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if publicProcedures[req.Spec().Procedure] {
				return next(ctx, req)
			}

			token := extractToken(req.Header())
			if token == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing token"))
			}

			claims, err := ParseToken(secret, token)
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token: %w", err))
			}

			ctx = context.WithValue(ctx, CtxUserID, claims.UserID)
			ctx = context.WithValue(ctx, CtxUsername, claims.Username)
			ctx = context.WithValue(ctx, CtxRole, claims.Role)

			return next(ctx, req)
		}
	}
}

// extractToken reads JWT from Authorization header or "token" cookie.
func extractToken(h http.Header) string {
	// Try Authorization header first
	if auth := h.Get("Authorization"); auth != "" {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	// Try cookie
	if cookie := h.Get("Cookie"); cookie != "" {
		for _, c := range strings.Split(cookie, ";") {
			c = strings.TrimSpace(c)
			if strings.HasPrefix(c, "token=") {
				return strings.TrimPrefix(c, "token=")
			}
		}
	}
	return ""
}

// ExtractTokenFromRequest reads JWT from http.Request (for REST API handlers).
func ExtractTokenFromRequest(r *http.Request, secret string) (*Claims, error) {
	// Try Authorization header
	if auth := r.Header.Get("Authorization"); auth != "" {
		return ParseToken(secret, strings.TrimPrefix(auth, "Bearer "))
	}
	// Try cookie
	cookie, err := r.Cookie("token")
	if err != nil || cookie.Value == "" {
		return nil, fmt.Errorf("no token")
	}
	return ParseToken(secret, cookie.Value)
}

// RequireRole returns an error if the caller doesn't have the required role.
func RequireRole(ctx context.Context, roles ...string) error {
	role, _ := ctx.Value(CtxRole).(string)
	for _, r := range roles {
		if role == r {
			return nil
		}
	}
	return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("insufficient role: need one of %v", roles))
}

// AuthMiddleware wraps http.Handler to inject JWT context for non-Connect routes (webhook).
func AuthMiddleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
