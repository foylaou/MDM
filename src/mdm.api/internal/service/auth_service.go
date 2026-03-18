package service

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"golang.org/x/crypto/argon2"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"

	mdmv1 "github.com/anthropics/mdm-server/gen/mdm/v1"
	"github.com/anthropics/mdm-server/gen/mdm/v1/mdmv1connect"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type AuthService struct {
	mdmv1connect.UnimplementedAuthServiceHandler
	users     port.UserRepository
	jwtSecret string
}

func NewAuthService(users port.UserRepository, jwtSecret string) *AuthService {
	return &AuthService{users: users, jwtSecret: jwtSecret}
}

func (s *AuthService) Login(ctx context.Context, req *connect.Request[mdmv1.LoginRequest]) (*connect.Response[mdmv1.LoginResponse], error) {
	user, err := s.users.GetByUsername(ctx, req.Msg.Username)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid credentials"))
	}

	if !verifyArgon2id(user.PasswordHash, req.Msg.Password) {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid credentials"))
	}

	access, refresh, expiresAt, err := middleware.GenerateTokens(s.jwtSecret, user.ID, user.Username, user.Role)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&mdmv1.LoginResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresAt:    expiresAt,
		User: &mdmv1.User{
			Id:          user.ID,
			Username:    user.Username,
			Role:        user.Role,
			DisplayName: user.DisplayName,
		},
	}), nil
}

func (s *AuthService) RefreshToken(ctx context.Context, req *connect.Request[mdmv1.RefreshTokenRequest]) (*connect.Response[mdmv1.RefreshTokenResponse], error) {
	claims, err := middleware.ParseToken(s.jwtSecret, req.Msg.RefreshToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid refresh token"))
	}

	access, refresh, expiresAt, err := middleware.GenerateTokens(s.jwtSecret, claims.UserID, claims.Username, claims.Role)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&mdmv1.RefreshTokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresAt:    expiresAt,
	}), nil
}

func (s *AuthService) ChangePassword(ctx context.Context, req *connect.Request[mdmv1.ChangePasswordRequest]) (*connect.Response[mdmv1.ChangePasswordResponse], error) {
	userID, _ := ctx.Value(middleware.CtxUserID).(string)
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}

	if !verifyArgon2id(user.PasswordHash, req.Msg.OldPassword) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("wrong current password"))
	}

	hash, err := hashArgon2id(req.Msg.NewPassword)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	user.PasswordHash = hash
	if err := s.users.Update(ctx, user); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&mdmv1.ChangePasswordResponse{}), nil
}

// HashArgon2id generates an Argon2id password hash.
func HashArgon2id(password string) (string, error) {
	return hashArgon2id(password)
}

// VerifyPassword checks a password against an Argon2id hash.
func VerifyPassword(hash, password string) bool {
	return verifyArgon2id(hash, password)
}

func hashArgon2id(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=4$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifyArgon2id(encoded, password string) bool {
	// Parse the encoded hash
	var version int
	var memory uint32
	var iterations uint32
	var parallelism uint8
	var salt, hash []byte

	_, err := fmt.Sscanf(encoded, "$argon2id$v=%d$m=%d,t=%d,p=%d$", &version, &memory, &iterations, &parallelism)
	if err != nil {
		return false
	}

	// Extract salt and hash parts
	parts := splitArgon2(encoded)
	if len(parts) != 2 {
		return false
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	hash, err = base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	computed := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(hash)))
	return subtle.ConstantTimeCompare(hash, computed) == 1
}

func splitArgon2(encoded string) []string {
	// Format: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
	parts := make([]string, 0, 2)
	count := 0
	for i := 0; i < len(encoded); i++ {
		if encoded[i] == '$' {
			count++
			if count == 4 {
				rest := encoded[i+1:]
				idx := 0
				for j := 0; j < len(rest); j++ {
					if rest[j] == '$' {
						parts = append(parts, rest[:j])
						idx = j + 1
						break
					}
				}
				if idx < len(rest) {
					parts = append(parts, rest[idx:])
				}
				return parts
			}
		}
	}
	return parts
}
