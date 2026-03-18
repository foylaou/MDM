package domain

import "time"

type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         string // "admin", "operator", "viewer"
	DisplayName  string
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Device struct {
	UDID             string
	SerialNumber     string
	DeviceName       string
	Model            string
	OSVersion        string
	LastSeen         time.Time
	EnrollmentStatus string
	IsSupervised     bool
	IsLostMode       bool
	BatteryLevel     float64
	Details          map[string]interface{} // cached command results (apps, profiles, security, etc.)
}

type AuditLog struct {
	ID        string
	UserID    string
	Username  string
	Action    string
	Target    string
	Detail    string
	Timestamp time.Time
}

type MDMEvent struct {
	ID          string
	EventType   string // "acknowledge", "checkin"
	UDID        string
	CommandUUID string
	Status      string
	RawPayload  string
	Timestamp   time.Time
}

type CommandResult struct {
	CommandUUID string
	StatusCode  int
	RawResponse string
}
