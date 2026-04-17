package domain

import "time"

type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         string // "admin", "operator", "viewer" (legacy)
	SystemRole   string // "sys_admin", "user"
	Email        string
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

// --- Asset Management ---

type Asset struct {
	ID            string
	DeviceUdid    *string
	AssetNumber   string
	Name          string
	Spec          string
	Quantity      int
	Unit          string
	AcquiredDate  *time.Time
	UnitPrice     float64
	Purpose       string
	BorrowDate    *time.Time
	CustodianID   *string
	CustodianName string
	Location      string
	AssetCategory string
	Notes         string
	CategoryID    *string
	AssetStatus   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	// Joined fields (read-only)
	DeviceName   string
	DeviceSerial string
	CategoryName string
}

// --- Rental Management ---

type Rental struct {
	ID              string
	DeviceUdid      string
	BorrowerID      string
	BorrowerName    string
	ApproverID      *string
	ApproverName    string
	CustodianID     *string
	CustodianName   string
	Status          string // pending, approved, active, returned, rejected
	Purpose         string
	BorrowDate      time.Time
	ExpectedReturn  *time.Time
	ActualReturn    *time.Time
	Notes           string
	RentalNumber    int
	IsArchived      bool
	ReturnChecklist map[string]interface{}
	ReturnNotes     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	// Joined fields (read-only)
	DeviceName   string
	DeviceSerial string
}

// --- App Management ---

type ManagedApp struct {
	ID            string
	Name          string
	BundleID      string
	AppType       string // "vpp", "enterprise"
	ItunesStoreID string
	ManifestURL   string
	PurchasedQty  int
	Notes         string
	IconURL       string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	// Computed
	InstalledCount int
}

type DeviceApp struct {
	ID          string
	DeviceUdid  string
	AppID       string
	InstalledAt time.Time
	// Joined fields
	AppName  string
	BundleID string
	AppType  string
}

type PendingAppCommand struct {
	CommandUUID string
	Action      string // "install", "uninstall"
	DeviceUdid  string
	AppID       string
}

// --- Category ---

type Category struct {
	ID        string
	ParentID  *string
	Name      string
	Level     int
	SortOrder int
	CreatedAt time.Time
}

// --- Profile ---

type Profile struct {
	ID         string
	Name       string
	Filename   string
	Content    []byte
	Size       int
	UploadedBy string
	CreatedAt  time.Time
}

// --- Module Permission ---

type ModulePermission struct {
	ID         string
	UserID     string
	Module     string // "asset", "mdm", "rental"
	Permission string // "viewer", "operator", "manager", "requester", "approver"
	GrantedBy  *string
	GrantedAt  time.Time
}

// --- Notification ---

type Notification struct {
	ID           string
	Type         string // "email"
	Event        string // "rental_request", "rental_approved", etc.
	Recipient    string // email address
	Subject      string
	Body         string
	Status       string // "pending", "sent", "failed"
	ErrorMessage string
	ReferenceID  string
	CreatedAt    time.Time
	SentAt       *time.Time
}

// --- Device List View (joined query) ---

type DeviceListItem struct {
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
	CustodianName    string
	CategoryName     string
	CategoryID       *string
	CustodianID      *string
	AssetStatus      string
}
