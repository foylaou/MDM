package service

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	mdmv1 "github.com/anthropics/mdm-server/gen/mdm/v1"
	"github.com/anthropics/mdm-server/gen/mdm/v1/mdmv1connect"
	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type UserService struct {
	mdmv1connect.UnimplementedUserServiceHandler
	users port.UserRepository
}

func NewUserService(users port.UserRepository) *UserService {
	return &UserService{users: users}
}

func (s *UserService) CreateUser(ctx context.Context, req *connect.Request[mdmv1.CreateUserRequest]) (*connect.Response[mdmv1.UserResponse], error) {
	if err := middleware.RequireRole(ctx, "admin"); err != nil {
		return nil, err
	}

	// Validate role
	switch req.Msg.Role {
	case "admin", "operator", "viewer":
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid role: %s (must be admin, operator, or viewer)", req.Msg.Role))
	}

	hash, err := hashArgon2id(req.Msg.Password)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	user := &domain.User{
		Username:     req.Msg.Username,
		PasswordHash: hash,
		Role:         req.Msg.Role,
		DisplayName:  req.Msg.DisplayName,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("user exists: %w", err))
	}
	return connect.NewResponse(&mdmv1.UserResponse{
		Id: user.ID, Username: user.Username, Role: user.Role, DisplayName: user.DisplayName,
	}), nil
}

func (s *UserService) ListUsers(ctx context.Context, req *connect.Request[mdmv1.ListUsersRequest]) (*connect.Response[mdmv1.ListUsersResponse], error) {
	if err := middleware.RequireRole(ctx, "admin"); err != nil {
		return nil, err
	}
	users, err := s.users.List(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &mdmv1.ListUsersResponse{}
	for _, u := range users {
		resp.Users = append(resp.Users, &mdmv1.UserResponse{
			Id: u.ID, Username: u.Username, Role: u.Role, DisplayName: u.DisplayName,
		})
	}
	return connect.NewResponse(resp), nil
}

func (s *UserService) UpdateUser(ctx context.Context, req *connect.Request[mdmv1.UpdateUserRequest]) (*connect.Response[mdmv1.UserResponse], error) {
	if err := middleware.RequireRole(ctx, "admin"); err != nil {
		return nil, err
	}
	user, err := s.users.GetByID(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if req.Msg.Role != "" {
		user.Role = req.Msg.Role
	}
	if req.Msg.DisplayName != "" {
		user.DisplayName = req.Msg.DisplayName
	}
	if err := s.users.Update(ctx, user); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&mdmv1.UserResponse{
		Id: user.ID, Username: user.Username, Role: user.Role, DisplayName: user.DisplayName,
	}), nil
}

func (s *UserService) DeleteUser(ctx context.Context, req *connect.Request[mdmv1.DeleteUserRequest]) (*connect.Response[mdmv1.DeleteUserResponse], error) {
	if err := middleware.RequireRole(ctx, "admin"); err != nil {
		return nil, err
	}
	if err := s.users.Delete(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&mdmv1.DeleteUserResponse{}), nil
}
