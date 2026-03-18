package service

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	mdmv1 "github.com/anthropics/mdm-server/gen/mdm/v1"
	"github.com/anthropics/mdm-server/gen/mdm/v1/mdmv1connect"
	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type DeviceService struct {
	mdmv1connect.UnimplementedDeviceServiceHandler
	mdm    port.MicroMDMClient
	repo   port.DeviceRepository
	audit  port.AuditRepository
}

func NewDeviceService(mdm port.MicroMDMClient, repo port.DeviceRepository, audit port.AuditRepository) *DeviceService {
	return &DeviceService{mdm: mdm, repo: repo, audit: audit}
}

func (s *DeviceService) ListDevices(ctx context.Context, req *connect.Request[mdmv1.ListDevicesRequest]) (*connect.Response[mdmv1.ListDevicesResponse], error) {
	limit := int(req.Msg.PageSize)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := 0
	if req.Msg.PageToken != "" {
		if v, err := strconv.Atoi(req.Msg.PageToken); err == nil && v > 0 {
			offset = v
		}
	}

	devices, total, err := s.repo.List(ctx, req.Msg.Filter, limit, offset)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := &mdmv1.ListDevicesResponse{TotalCount: int32(total)}
	for _, d := range devices {
		resp.Devices = append(resp.Devices, &mdmv1.Device{
			Udid:             d.UDID,
			SerialNumber:     d.SerialNumber,
			DeviceName:       d.DeviceName,
			Model:            d.Model,
			OsVersion:        d.OSVersion,
			LastSeen:         timestamppb.New(d.LastSeen),
			EnrollmentStatus: d.EnrollmentStatus,
		})
	}
	return connect.NewResponse(resp), nil
}

func (s *DeviceService) GetDevice(ctx context.Context, req *connect.Request[mdmv1.GetDeviceRequest]) (*connect.Response[mdmv1.Device], error) {
	d, err := s.repo.GetByUDID(ctx, req.Msg.Udid)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&mdmv1.Device{
		Udid:             d.UDID,
		SerialNumber:     d.SerialNumber,
		DeviceName:       d.DeviceName,
		Model:            d.Model,
		OsVersion:        d.OSVersion,
		LastSeen:         timestamppb.New(d.LastSeen),
		EnrollmentStatus: d.EnrollmentStatus,
	}), nil
}

func (s *DeviceService) SyncDevices(ctx context.Context, req *connect.Request[mdmv1.SyncDevicesRequest]) (*connect.Response[mdmv1.SyncDevicesResponse], error) {
	if err := middleware.RequireRole(ctx, "admin", "operator"); err != nil {
		return nil, err
	}

	devices, err := s.mdm.ListDevices(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, d := range devices {
		if err := s.repo.Upsert(ctx, d); err != nil {
			log.Printf("device upsert %s: %v", d.UDID, err)
		}
	}

	username, _ := ctx.Value(middleware.CtxUsername).(string)
	userID, _ := ctx.Value(middleware.CtxUserID).(string)
	if err := s.audit.Create(ctx, &domain.AuditLog{
		UserID: userID, Username: username, Action: "sync_devices", Detail: fmt.Sprintf("synced %d devices", len(devices)),
	}); err != nil {
		log.Printf("audit log: %v", err)
	}

	// Send DeviceInformation query to each device to get full info
	go func() {
		bgCtx := context.Background()
		queries := deviceInfoQueries()
		for _, d := range devices {
			payload := map[string]interface{}{
				"udid":         d.UDID,
				"request_type": "DeviceInformation",
				"queries":      queries,
			}
			if _, err := s.mdm.SendCommand(bgCtx, payload); err != nil {
				log.Printf("device info query %s: %v", d.UDID, err)
				continue
			}
			_ = s.mdm.SendPush(bgCtx, d.UDID)
		}
		log.Printf("sent DeviceInformation to %d devices", len(devices))
	}()

	return connect.NewResponse(&mdmv1.SyncDevicesResponse{SyncedCount: int32(len(devices))}), nil
}

func deviceInfoQueries() []string {
	return []string{
		"UDID", "DeviceName", "OSVersion", "BuildVersion",
		"ModelName", "Model", "ProductName", "SerialNumber",
		"DeviceCapacity", "AvailableDeviceCapacity", "BatteryLevel",
		"IsSupervised", "IsActivationLockEnabled", "IsMDMLostModeEnabled",
		"WiFiMAC", "BluetoothMAC", "IMEI",
	}
}

func (s *DeviceService) SyncDEPDevices(ctx context.Context, req *connect.Request[mdmv1.SyncDEPDevicesRequest]) (*connect.Response[mdmv1.SyncDEPDevicesResponse], error) {
	if err := middleware.RequireRole(ctx, "admin"); err != nil {
		return nil, err
	}
	if err := s.mdm.SyncDEP(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&mdmv1.SyncDEPDevicesResponse{Status: "ok"}), nil
}
