package port

import (
	"context"

	"github.com/anthropics/mdm-server/internal/domain"
)

// UserRepository persists users.
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	List(ctx context.Context) ([]*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, id string) error
}

// DeviceRepository persists devices.
type DeviceRepository interface {
	Upsert(ctx context.Context, device *domain.Device) error
	GetByUDID(ctx context.Context, udid string) (*domain.Device, error)
	List(ctx context.Context, filter string, limit int, offset int) ([]*domain.Device, int, error)
}

// AuditRepository persists audit logs.
type AuditRepository interface {
	Create(ctx context.Context, log *domain.AuditLog) error
	List(ctx context.Context, userID string, action string, limit int, offset int) ([]*domain.AuditLog, error)
}

// MicroMDMClient calls the MicroMDM HTTP API.
type MicroMDMClient interface {
	ListDevices(ctx context.Context) ([]*domain.Device, error)
	GetDevice(ctx context.Context, udid string) (*domain.Device, error)
	SendCommand(ctx context.Context, payload map[string]interface{}) (*domain.CommandResult, error)
	SendPush(ctx context.Context, udid string) error
	ClearQueue(ctx context.Context, udid string) (*domain.CommandResult, error)
	InspectQueue(ctx context.Context, udid string) (string, error)
	SyncDEP(ctx context.Context) error
}

// VPPClient calls the Apple VPP API.
type VPPClient interface {
	AssignLicense(ctx context.Context, adamID string, serialNumbers []string) (string, error)
	RevokeLicense(ctx context.Context, adamID string, serialNumbers []string) (string, error)
}

// AssetRepository checks asset ownership (custodian).
type AssetRepository interface {
	IsCustodianOfAll(ctx context.Context, userID string, udids []string) (bool, error)
}

// EventBroker fans out MDM events to subscribers.
type EventBroker interface {
	Publish(event *domain.MDMEvent)
	Subscribe(ctx context.Context) <-chan *domain.MDMEvent
}
