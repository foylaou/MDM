package service

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	mdmv1 "github.com/anthropics/mdm-server/gen/mdm/v1"
	"github.com/anthropics/mdm-server/gen/mdm/v1/mdmv1connect"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type AuditService struct {
	mdmv1connect.UnimplementedAuditServiceHandler
	repo port.AuditRepository
}

func NewAuditService(repo port.AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

func (s *AuditService) ListAuditLogs(ctx context.Context, req *connect.Request[mdmv1.ListAuditLogsRequest]) (*connect.Response[mdmv1.ListAuditLogsResponse], error) {
	if err := middleware.RequireRole(ctx, "admin"); err != nil {
		return nil, err
	}
	limit := int(req.Msg.PageSize)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	logs, err := s.repo.List(ctx, req.Msg.UserId, req.Msg.Action, limit, 0)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &mdmv1.ListAuditLogsResponse{}
	for _, l := range logs {
		resp.Logs = append(resp.Logs, &mdmv1.AuditLog{
			Id: l.ID, UserId: l.UserID, Username: l.Username,
			Action: l.Action, Target: l.Target, Detail: l.Detail,
			Timestamp: timestamppb.New(l.Timestamp),
		})
	}
	return connect.NewResponse(resp), nil
}
