package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"connectrpc.com/connect"

	mdmv1 "github.com/anthropics/mdm-server/gen/mdm/v1"
	"github.com/anthropics/mdm-server/gen/mdm/v1/mdmv1connect"
	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type CommandService struct {
	mdmv1connect.UnimplementedCommandServiceHandler
	mdm     port.MicroMDMClient
	vpp     port.VPPClient
	audit   port.AuditRepository
	broker  port.EventBroker
	assets  port.AssetRepository
	devices port.DeviceRepository
}

func NewCommandService(mdm port.MicroMDMClient, vpp port.VPPClient, audit port.AuditRepository, broker port.EventBroker, assets port.AssetRepository, devices port.DeviceRepository) *CommandService {
	return &CommandService{mdm: mdm, vpp: vpp, audit: audit, broker: broker, assets: assets, devices: devices}
}

func (s *CommandService) requireRoleOrCustodian(ctx context.Context, udids []string) error {
	// Admin and operator always pass
	if err := middleware.RequireRole(ctx, "admin", "operator"); err == nil {
		return nil
	}
	// Otherwise check if the user is the custodian of all target devices
	userID, _ := ctx.Value(middleware.CtxUserID).(string)
	if userID == "" {
		return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("insufficient permissions"))
	}
	ok, err := s.assets.IsCustodianOfAll(ctx, userID, udids)
	if err != nil {
		log.Printf("custodian check error: %v", err)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("permission check failed"))
	}
	if !ok {
		return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("insufficient permissions: not admin/operator or custodian of target devices"))
	}
	return nil
}

func (s *CommandService) sendToAll(ctx context.Context, udids []string, payload func(udid string) map[string]interface{}) (*connect.Response[mdmv1.CommandResponse], error) {
	if err := s.requireRoleOrCustodian(ctx, udids); err != nil {
		return nil, err
	}
	if len(udids) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("no devices specified"))
	}

	var lastResult *domain.CommandResult
	for _, udid := range udids {
		p := payload(udid)
		result, err := s.mdm.SendCommand(ctx, p)
		if err != nil {
			s.publishEvent(udid, "", p, "error", err.Error())
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("command to %s: %w", udid, err))
		}

		s.publishEvent(udid, result.CommandUUID, p, "sent", result.RawResponse)

		if err := s.mdm.SendPush(ctx, udid); err != nil {
			log.Printf("push %s: %v", udid, err)
		}
		lastResult = result
	}

	return connect.NewResponse(&mdmv1.CommandResponse{
		CommandUuid: lastResult.CommandUUID,
		StatusCode:  int32(lastResult.StatusCode),
		RawResponse: lastResult.RawResponse,
	}), nil
}

func (s *CommandService) publishEvent(udid, commandUUID string, payload map[string]interface{}, status string, detail string) {
	requestType, _ := payload["request_type"].(string)
	raw, _ := json.Marshal(payload)
	s.broker.Publish(&domain.MDMEvent{
		ID:          fmt.Sprintf("cmd-%d", time.Now().UnixNano()),
		EventType:   "command_" + status,
		UDID:        udid,
		CommandUUID: commandUUID,
		Status:      status,
		RawPayload:  string(raw),
		Timestamp:   time.Now(),
	})
	log.Printf("[command] %s → %s udid=%s cmd=%s status=%s", requestType, udid, udid[:8], commandUUID, status)
}

func (s *CommandService) auditAction(ctx context.Context, action, target, detail string) {
	userID, _ := ctx.Value(middleware.CtxUserID).(string)
	username, _ := ctx.Value(middleware.CtxUsername).(string)
	if err := s.audit.Create(ctx, &domain.AuditLog{
		UserID: userID, Username: username, Action: action, Target: target, Detail: detail,
	}); err != nil {
		log.Printf("audit log: %v", err)
	}
}

func (s *CommandService) LockDevice(ctx context.Context, req *connect.Request[mdmv1.LockDeviceRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "lock_device", fmt.Sprint(req.Msg.Udids), req.Msg.Message)
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		p := map[string]interface{}{"udid": udid, "request_type": "DeviceLock"}
		if req.Msg.Pin != "" {
			p["pin"] = req.Msg.Pin
		}
		if req.Msg.Message != "" {
			p["message"] = req.Msg.Message
		}
		return p
	})
}

func (s *CommandService) RestartDevice(ctx context.Context, req *connect.Request[mdmv1.RestartDeviceRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "restart_device", fmt.Sprint(req.Msg.Udids), "")
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "RestartDevice"}
	})
}

func (s *CommandService) ShutdownDevice(ctx context.Context, req *connect.Request[mdmv1.ShutdownDeviceRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "shutdown_device", fmt.Sprint(req.Msg.Udids), "")
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "ShutDownDevice"}
	})
}

func (s *CommandService) EraseDevice(ctx context.Context, req *connect.Request[mdmv1.EraseDeviceRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	if err := middleware.RequireRole(ctx, "admin"); err != nil {
		return nil, err
	}
	s.auditAction(ctx, "erase_device", fmt.Sprint(req.Msg.Udids), "")
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		p := map[string]interface{}{"udid": udid, "request_type": "EraseDevice"}
		if req.Msg.Pin != "" {
			p["pin"] = req.Msg.Pin
		}
		return p
	})
}

func (s *CommandService) ClearPasscode(ctx context.Context, req *connect.Request[mdmv1.ClearPasscodeRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "clear_passcode", fmt.Sprint(req.Msg.Udids), "")
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "ClearPasscode"}
	})
}

func (s *CommandService) InstallApp(ctx context.Context, req *connect.Request[mdmv1.InstallAppRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "install_app", fmt.Sprint(req.Msg.Udids), req.Msg.ItunesStoreId)

	// Optionally assign VPP licenses first
	if req.Msg.AssignVppLicense && s.vpp != nil {
		// We need serial numbers but only have UDIDs - in production you'd look these up
		// For now, pass UDIDs and let VPP adapter handle
	}

	storeID, err := strconv.ParseInt(req.Msg.ItunesStoreId, 10, 64)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid itunes_store_id: %w", err))
	}

	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{
			"udid":            udid,
			"request_type":    "InstallApplication",
			"itunes_store_id": storeID,
			"options":         map[string]interface{}{"purchase_method": 1},
		}
	})
}

func (s *CommandService) InstallEnterpriseApp(ctx context.Context, req *connect.Request[mdmv1.InstallEnterpriseAppRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "install_enterprise_app", fmt.Sprint(req.Msg.Udids), req.Msg.ManifestUrl)
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{
			"udid":         udid,
			"request_type": "InstallEnterpriseApplication",
			"manifest_url": req.Msg.ManifestUrl,
		}
	})
}

func (s *CommandService) RemoveApp(ctx context.Context, req *connect.Request[mdmv1.RemoveAppRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "remove_app", fmt.Sprint(req.Msg.Udids), req.Msg.Identifier)
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{
			"udid":         udid,
			"request_type": "RemoveApplication",
			"identifier":   req.Msg.Identifier,
		}
	})
}

func (s *CommandService) InstallProfile(ctx context.Context, req *connect.Request[mdmv1.InstallProfileRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "install_profile", fmt.Sprint(req.Msg.Udids), "")
	encoded := base64.StdEncoding.EncodeToString(req.Msg.Payload)
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{
			"udid":         udid,
			"request_type": "InstallProfile",
			"payload":      encoded,
		}
	})
}

func (s *CommandService) RemoveProfile(ctx context.Context, req *connect.Request[mdmv1.RemoveProfileRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "remove_profile", fmt.Sprint(req.Msg.Udids), req.Msg.Identifier)
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{
			"udid":         udid,
			"request_type": "RemoveProfile",
			"identifier":   req.Msg.Identifier,
		}
	})
}

func (s *CommandService) GetDeviceInfo(ctx context.Context, req *connect.Request[mdmv1.GetDeviceInfoRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{
			"udid":         udid,
			"request_type": "DeviceInformation",
			"queries": []string{
				"UDID", "DeviceName", "OSVersion", "BuildVersion",
				"ModelName", "Model", "ProductName", "SerialNumber",
				"DeviceCapacity", "AvailableDeviceCapacity", "BatteryLevel",
				"IsSupervised", "IsActivationLockEnabled", "IsMDMLostModeEnabled",
				"IsDeviceLocatorServiceEnabled", "IsCloudBackupEnabled", "IsDoNotDisturbInEffect",
				"WiFiMAC", "BluetoothMAC", "EthernetMACs",
				"ICCID", "IMEI", "MEID", "ModemFirmwareVersion",
				"CellularTechnology", "CurrentCarrierNetwork", "SubscriberCarrierNetwork",
				"CurrentMCC", "CurrentMNC", "PhoneNumber",
				"DataRoamingEnabled", "VoiceRoamingEnabled", "PersonalHotspotEnabled", "IsRoaming",
				"EASDeviceIdentifier", "IsMultiUser", "IsNetworkTethered",
				"AwaitingConfiguration", "LastCloudBackupDate",
				"iTunesStoreAccountIsActive", "iTunesStoreAccountHash",
				"OrganizationInfo", "MDMOptions", "PushToken",
				"OSUpdateSettings", "AutomaticCheckEnabled", "BackgroundDownloadEnabled",
				"AutomaticAppInstallationEnabled", "AutomaticOSInstallationEnabled",
				"AutomaticSecurityUpdatesEnabled",
				"DiagnosticSubmissionEnabled", "AppAnalyticsEnabled",
				"Languages", "Locales", "DeviceID",
				"ServiceSubscriptions", "MaximumResidentUsers",
			},
		}
	})
}

func (s *CommandService) GetInstalledApps(ctx context.Context, req *connect.Request[mdmv1.GetInstalledAppsRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "InstalledApplicationList"}
	})
}

func (s *CommandService) GetProfileList(ctx context.Context, req *connect.Request[mdmv1.GetProfileListRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "ProfileList"}
	})
}

func (s *CommandService) GetSecurityInfo(ctx context.Context, req *connect.Request[mdmv1.GetSecurityInfoRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "SecurityInfo"}
	})
}

func (s *CommandService) GetCertificateList(ctx context.Context, req *connect.Request[mdmv1.GetCertificateListRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "CertificateList"}
	})
}

func (s *CommandService) GetAvailableOSUpdates(ctx context.Context, req *connect.Request[mdmv1.GetAvailableOSUpdatesRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "AvailableOSUpdates"}
	})
}

func (s *CommandService) ScheduleOSUpdate(ctx context.Context, req *connect.Request[mdmv1.ScheduleOSUpdateRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "schedule_os_update", fmt.Sprint(req.Msg.Udids), req.Msg.ProductVersion)
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{
			"udid":         udid,
			"request_type": "ScheduleOSUpdate",
			"updates": []map[string]interface{}{{
				"install_action":     req.Msg.InstallAction,
				"product_key":        req.Msg.ProductKey,
				"product_version":    req.Msg.ProductVersion,
				"max_user_deferrals": 1,
				"priority":           "High",
			}},
		}
	})
}

func (s *CommandService) SetupAccount(ctx context.Context, req *connect.Request[mdmv1.SetupAccountRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "setup_account", fmt.Sprint(req.Msg.Udids), req.Msg.UserName)
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{
			"udid":                                udid,
			"request_type":                        "AccountConfiguration",
			"skip_primary_setup_account_creation": false,
			"set_primary_setup_account_as_regular_user": false,
			"dont_auto_populate_primary_account_info":   false,
			"lock_primary_account_info":                 req.Msg.LockAccountInfo,
			"primary_account_full_name":                 req.Msg.FullName,
			"primary_account_user_name":                 req.Msg.UserName,
		}
	})
}

func (s *CommandService) DeviceConfigured(ctx context.Context, req *connect.Request[mdmv1.DeviceConfiguredRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "device_configured", fmt.Sprint(req.Msg.Udids), "")
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{
			"udid":                            udid,
			"request_type":                    "DeviceConfigured",
			"request_requires_network_tether": false,
		}
	})
}

func (s *CommandService) GetActivationLockBypass(ctx context.Context, req *connect.Request[mdmv1.GetActivationLockBypassRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "ActivationLockBypassCode"}
	})
}

func (s *CommandService) EnableLostMode(ctx context.Context, req *connect.Request[mdmv1.EnableLostModeRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "enable_lost_mode", fmt.Sprint(req.Msg.Udids), req.Msg.Message)
	resp, err := s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		p := map[string]interface{}{"udid": udid, "request_type": "EnableLostMode"}
		if req.Msg.Message != "" {
			p["message"] = req.Msg.Message
		}
		if req.Msg.PhoneNumber != "" {
			p["phone_number"] = req.Msg.PhoneNumber
		}
		if req.Msg.Footnote != "" {
			p["footnote"] = req.Msg.Footnote
		}
		return p
	})
	if err == nil && s.devices != nil {
		for _, udid := range req.Msg.Udids {
			_ = s.devices.SetLostMode(ctx, udid, true)
		}
	}
	return resp, err
}

func (s *CommandService) DisableLostMode(ctx context.Context, req *connect.Request[mdmv1.DisableLostModeRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "disable_lost_mode", fmt.Sprint(req.Msg.Udids), "")
	resp, err := s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "DisableLostMode"}
	})
	if err == nil && s.devices != nil {
		for _, udid := range req.Msg.Udids {
			_ = s.devices.SetLostMode(ctx, udid, false)
		}
	}
	return resp, err
}

func (s *CommandService) GetDeviceLocation(ctx context.Context, req *connect.Request[mdmv1.GetDeviceLocationRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "DeviceLocation"}
	})
}

func (s *CommandService) PlayLostModeSound(ctx context.Context, req *connect.Request[mdmv1.PlayLostModeSoundRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	return s.sendToAll(ctx, req.Msg.Udids, func(udid string) map[string]interface{} {
		return map[string]interface{}{"udid": udid, "request_type": "PlayLostModeSound"}
	})
}

func (s *CommandService) SendPush(ctx context.Context, req *connect.Request[mdmv1.SendPushRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	for _, udid := range req.Msg.Udids {
		if err := s.mdm.SendPush(ctx, udid); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("push %s: %w", udid, err))
		}
	}
	return connect.NewResponse(&mdmv1.CommandResponse{StatusCode: 200}), nil
}

func (s *CommandService) ClearCommandQueue(ctx context.Context, req *connect.Request[mdmv1.ClearCommandQueueRequest]) (*connect.Response[mdmv1.CommandResponse], error) {
	s.auditAction(ctx, "clear_queue", fmt.Sprint(req.Msg.Udids), "")
	var lastResult *domain.CommandResult
	for _, udid := range req.Msg.Udids {
		r, err := s.mdm.ClearQueue(ctx, udid)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		lastResult = r
	}
	if lastResult == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("no devices"))
	}
	return connect.NewResponse(&mdmv1.CommandResponse{
		StatusCode: int32(lastResult.StatusCode), RawResponse: lastResult.RawResponse,
	}), nil
}

func (s *CommandService) InspectCommandQueue(ctx context.Context, req *connect.Request[mdmv1.InspectCommandQueueRequest]) (*connect.Response[mdmv1.InspectCommandQueueResponse], error) {
	raw, err := s.mdm.InspectQueue(ctx, req.Msg.Udid)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&mdmv1.InspectCommandQueueResponse{
		Udid: req.Msg.Udid, RawResponse: raw,
	}), nil
}
