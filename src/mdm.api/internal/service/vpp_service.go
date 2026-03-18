package service

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	mdmv1 "github.com/anthropics/mdm-server/gen/mdm/v1"
	"github.com/anthropics/mdm-server/gen/mdm/v1/mdmv1connect"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type VPPService struct {
	mdmv1connect.UnimplementedVPPServiceHandler
	vpp port.VPPClient
}

func NewVPPService(vpp port.VPPClient) *VPPService {
	return &VPPService{vpp: vpp}
}

func (s *VPPService) AssignLicense(ctx context.Context, req *connect.Request[mdmv1.AssignLicenseRequest]) (*connect.Response[mdmv1.AssignLicenseResponse], error) {
	if err := middleware.RequireRole(ctx, "admin", "operator"); err != nil {
		return nil, err
	}
	raw, err := s.vpp.AssignLicense(ctx, req.Msg.AdamId, req.Msg.SerialNumbers)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("vpp assign: %w", err))
	}
	return connect.NewResponse(&mdmv1.AssignLicenseResponse{Status: "ok", RawResponse: raw}), nil
}

func (s *VPPService) RevokeLicense(ctx context.Context, req *connect.Request[mdmv1.RevokeLicenseRequest]) (*connect.Response[mdmv1.RevokeLicenseResponse], error) {
	if err := middleware.RequireRole(ctx, "admin", "operator"); err != nil {
		return nil, err
	}
	raw, err := s.vpp.RevokeLicense(ctx, req.Msg.AdamId, req.Msg.SerialNumbers)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("vpp revoke: %w", err))
	}
	return connect.NewResponse(&mdmv1.RevokeLicenseResponse{Status: "ok", RawResponse: raw}), nil
}
