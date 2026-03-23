package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/anthropics/mdm-server/gen/mdm/v1/mdmv1connect"
	"github.com/anthropics/mdm-server/internal/adapter/micromdm"
	"github.com/anthropics/mdm-server/internal/adapter/postgres"
	"github.com/anthropics/mdm-server/internal/adapter/vpp"
	"github.com/anthropics/mdm-server/internal/config"
	"github.com/anthropics/mdm-server/internal/db"
	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/service"
)

func main() {
	cfg := config.Load()

	// Database
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	// Verify database connectivity
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("database ping failed: %v", err)
	}

	// Run migrations
	runMigrations(pool)

	// Backfill app icons from iTunes API for existing apps
	go backfillAppIcons(pool)

	// Adapters
	mdmClient := micromdm.NewClient(cfg.MicroMDMURL, cfg.MicroMDMKey)
	vppClient, err := vpp.NewClient(cfg.VPPTokenPath)
	if err != nil {
		log.Printf("VPP client not configured: %v", err)
	}

	// Repositories
	userRepo := postgres.NewUserRepo(pool)
	deviceRepo := postgres.NewDeviceRepo(pool)
	auditRepo := postgres.NewAuditRepo(pool)
	assetRepo := postgres.NewAssetRepo(pool)

	// Event broker
	broker := service.NewEventBroker()

	// Services
	authSvc := service.NewAuthService(userRepo, cfg.JWTSecret)
	deviceSvc := service.NewDeviceService(mdmClient, deviceRepo, auditRepo)
	commandSvc := service.NewCommandService(mdmClient, vppClient, auditRepo, broker, assetRepo, deviceRepo)
	eventSvc := service.NewEventService(broker)
	vppSvc := service.NewVPPService(vppClient)
	userSvc := service.NewUserService(userRepo)
	auditSvc := service.NewAuditService(auditRepo)

	// Interceptors
	interceptors := connect.WithInterceptors(middleware.NewAuthInterceptor(cfg.JWTSecret))

	// Mux
	mux := http.NewServeMux()

	// Register ConnectRPC services
	path, handler := mdmv1connect.NewAuthServiceHandler(authSvc, interceptors)
	mux.Handle(path, handler)

	path, handler = mdmv1connect.NewDeviceServiceHandler(deviceSvc, interceptors)
	mux.Handle(path, handler)

	path, handler = mdmv1connect.NewCommandServiceHandler(commandSvc, interceptors)
	mux.Handle(path, handler)

	path, handler = mdmv1connect.NewEventServiceHandler(eventSvc)
	mux.Handle(path, handler)

	path, handler = mdmv1connect.NewVPPServiceHandler(vppSvc, interceptors)
	mux.Handle(path, handler)

	path, handler = mdmv1connect.NewUserServiceHandler(userSvc, interceptors)
	mux.Handle(path, handler)

	path, handler = mdmv1connect.NewAuditServiceHandler(auditSvc, interceptors)
	mux.Handle(path, handler)

	// Webhook endpoint (no auth - MicroMDM calls this)
	webhookHandler := service.NewWebhookHandler(broker, deviceRepo)
	mux.Handle(cfg.WebhookPath, webhookHandler)

	// SocketIO relay — connects to external SocketIO server and forwards events
	if cfg.WebSocketURL != "" {
		relay := service.NewSocketIORelay(cfg.WebSocketURL, cfg.MicroMDMKey, webhookHandler)
		relay.Start()
	}

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// REST Login — sets HttpOnly cookie
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		user, err := userRepo.GetByUsername(r.Context(), body.Username)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid credentials"}`))
			return
		}
		if !service.VerifyPassword(user.PasswordHash, body.Password) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid credentials"}`))
			return
		}
		if !user.IsActive {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"account pending activation","code":"inactive"}`))
			return
		}
		access, _, expiresAt, err := middleware.GenerateTokens(cfg.JWTSecret, user.ID, user.Username, user.Role)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     middleware.CookieName,
			Value:    access,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   24 * 60 * 60, // 24h
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"expires_at": expiresAt,
			"user": map[string]string{
				"id": user.ID, "username": user.Username,
				"role": user.Role, "display_name": user.DisplayName,
			},
		})
	})

	// REST Logout — clears cookie
	mux.HandleFunc("/api/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name: middleware.CookieName, Value: "", Path: "/",
			HttpOnly: true, MaxAge: -1,
		})
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	// REST /api/me — check auth status
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"id": claims.UserID, "username": claims.Username, "role": claims.Role,
		})
	})

	// Public register — creates inactive user, needs admin activation
	mux.HandleFunc("/api/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Username    string `json:"username"`
			Password    string `json:"password"`
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"username and password required"}`))
			return
		}
		if len(body.Password) < 6 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"password must be at least 6 characters"}`))
			return
		}
		hash, err := service.HashArgon2id(body.Password)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		displayName := body.DisplayName
		if displayName == "" {
			displayName = body.Username
		}
		_, err = pool.Exec(r.Context(),
			`INSERT INTO users (username, password_hash, role, display_name, is_active) VALUES ($1, $2, 'viewer', $3, false)`,
			body.Username, hash, displayName)
		if err != nil {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error":"username already exists"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"message":"registration successful, please wait for admin activation"}`))
		log.Printf("[register] new user '%s' registered (inactive)", body.Username)
	})

	// System status - check if initialized (public, no auth)
	mux.HandleFunc("/api/system-status", func(w http.ResponseWriter, r *http.Request) {
		var count int
		err := pool.QueryRow(r.Context(), "SELECT count(*) FROM users").Scan(&count)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"database error"}`))
			return
		}
		if count == 0 {
			w.Write([]byte(`{"initialized":false}`))
		} else {
			w.Write([]byte(`{"initialized":true}`))
		}
	})

	// Initial setup - create first admin user (only works when no users exist)
	mux.HandleFunc("/api/setup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var count int
		if err := pool.QueryRow(r.Context(), "SELECT count(*) FROM users").Scan(&count); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"database error"}`))
			return
		}
		if count > 0 {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"system already initialized"}`))
			return
		}
		var body struct {
			Username    string `json:"username"`
			Password    string `json:"password"`
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"username and password required"}`))
			return
		}
		hash, err := service.HashArgon2id(body.Password)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"failed to hash password"}`))
			return
		}
		displayName := body.DisplayName
		if displayName == "" {
			displayName = body.Username
		}
		_, err = pool.Exec(r.Context(),
			`INSERT INTO users (username, password_hash, role, display_name) VALUES ($1, $2, 'admin', $3)`,
			body.Username, hash, displayName)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"failed to create user"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
		log.Printf("setup: admin user '%s' created", body.Username)
	})

	// WebSocket config — frontend checks if SocketIO relay is available
	// API key is NOT exposed to frontend — Go backend handles SocketIO directly
	mux.HandleFunc("/api/ws-config", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backend_relay": cfg.WebSocketURL != "",
		})
	})

	// Device details API — GET returns full device info + cached details from DB
	mux.HandleFunc("/api/devices/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		udid := strings.TrimPrefix(r.URL.Path, "/api/devices/")
		if udid == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if r.Method == http.MethodGet {
			d, err := deviceRepo.GetByUDID(r.Context(), udid)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"not found"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"udid":              d.UDID,
				"serial_number":     d.SerialNumber,
				"device_name":       d.DeviceName,
				"model":             d.Model,
				"os_version":        d.OSVersion,
				"last_seen":         d.LastSeen.Format(time.RFC3339),
				"enrollment_status": d.EnrollmentStatus,
				"is_supervised":     d.IsSupervised,
				"is_lost_mode":      d.IsLostMode,
				"battery_level":     d.BatteryLevel,
				"details":           d.Details,
			})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// Device list with asset info (for frontend Devices page)
	mux.HandleFunc("/api/devices-list", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		filter := r.URL.Query().Get("filter")
		category := r.URL.Query().Get("category_id")
		custodian := r.URL.Query().Get("custodian_id")
		rentalStatus := r.URL.Query().Get("rental_status")

		q := `SELECT d.udid, d.serial_number, d.device_name, d.model, d.os_version,
		             d.last_seen, d.enrollment_status, d.is_supervised, d.is_lost_mode, d.battery_level,
		             COALESCE(a.custodian_name,'') as custodian_name,
		             COALESCE(c.name,'') as category_name,
		             a.category_id, a.custodian_id,
		             COALESCE(a.asset_status,'available') as asset_status,
		             EXISTS(SELECT 1 FROM rentals rl WHERE rl.device_udid = d.udid AND rl.status = 'active') as is_rented
		      FROM devices d
		      LEFT JOIN assets a ON a.device_udid = d.udid
		      LEFT JOIN categories c ON a.category_id = c.id
		      WHERE 1=1`
		args := []interface{}{}
		idx := 1
		if filter != "" {
			q += fmt.Sprintf(` AND (d.serial_number ILIKE $%d OR d.device_name ILIKE $%d OR d.udid ILIKE $%d)`, idx, idx, idx)
			args = append(args, "%"+filter+"%")
			idx++
		}
		if category != "" {
			q += fmt.Sprintf(` AND a.category_id = $%d`, idx)
			args = append(args, category)
			idx++
		}
		if custodian != "" {
			q += fmt.Sprintf(` AND a.custodian_id = $%d`, idx)
			args = append(args, custodian)
			idx++
		}
		if rentalStatus != "" {
			q += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM rentals rl WHERE rl.device_udid = d.udid AND rl.status = $%d)`, idx)
			args = append(args, rentalStatus)
			idx++
		}
		// Viewer: only show devices they are currently borrowing
		if claims.Role == "viewer" {
			q += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM rentals rl WHERE rl.device_udid = d.udid AND rl.borrower_id = $%d AND rl.status = 'active')`, idx)
			args = append(args, claims.UserID)
			idx++
		}
		q += ` ORDER BY d.last_seen DESC`

		rows, err := pool.Query(r.Context(), q, args...)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("devices-list: %v", err)
			return
		}
		defer rows.Close()

		type deviceRow struct {
			UDID             string  `json:"udid"`
			SerialNumber     string  `json:"serial_number"`
			DeviceName       string  `json:"device_name"`
			Model            string  `json:"model"`
			OSVersion        string  `json:"os_version"`
			LastSeen         string  `json:"last_seen"`
			EnrollmentStatus string  `json:"enrollment_status"`
			IsSupervised     bool    `json:"is_supervised"`
			IsLostMode       bool    `json:"is_lost_mode"`
			BatteryLevel     float64 `json:"battery_level"`
			CustodianName    string  `json:"custodian_name"`
			CategoryName     string  `json:"category_name"`
			CategoryID       *string `json:"category_id"`
			CustodianID      *string `json:"custodian_id"`
			AssetStatus      string  `json:"asset_status"`
		}
		var devices []deviceRow
		for rows.Next() {
			var d deviceRow
			var lastSeen time.Time
			var assetStatus string
			var isRented bool
			if err := rows.Scan(&d.UDID, &d.SerialNumber, &d.DeviceName, &d.Model, &d.OSVersion,
				&lastSeen, &d.EnrollmentStatus, &d.IsSupervised, &d.IsLostMode, &d.BatteryLevel,
				&d.CustodianName, &d.CategoryName, &d.CategoryID, &d.CustodianID,
				&assetStatus, &isRented); err != nil {
				log.Printf("devices-list scan: %v", err)
				continue
			}
			d.LastSeen = lastSeen.Format(time.RFC3339)
			// Compute effective status: auto-detect rented/lost, otherwise use manual status
			if isRented {
				d.AssetStatus = "rented"
			} else if d.IsLostMode {
				d.AssetStatus = "lost"
			} else {
				d.AssetStatus = assetStatus
			}
			devices = append(devices, d)
		}
		if devices == nil {
			devices = []deviceRow{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"devices": devices, "total": len(devices)})
	})

	// Update device asset_status by UDID
	mux.HandleFunc("/api/device-status", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var body struct {
			UDID   string `json:"udid"`
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UDID == "" || body.Status == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"udid and status required"}`))
			return
		}
		valid := map[string]bool{"available": true, "faulty": true, "repairing": true, "retired": true}
		if !valid[body.Status] {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid status"})
			return
		}
		_, err := pool.Exec(r.Context(),
			`UPDATE assets SET asset_status=$1, updated_at=now() WHERE device_udid=$2`, body.Status, body.UDID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Write([]byte(`{"ok":true}`))
	})

	// Batch sync device info — send DeviceInformation to all devices
	mux.HandleFunc("/api/sync-device-info", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		devices, _, err := deviceRepo.List(r.Context(), "", 500, 0)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		queries := []string{
			"UDID", "DeviceName", "OSVersion", "BuildVersion",
			"ModelName", "Model", "ProductName", "SerialNumber",
			"DeviceCapacity", "AvailableDeviceCapacity", "BatteryLevel",
			"IsSupervised", "IsActivationLockEnabled", "IsMDMLostModeEnabled",
			"WiFiMAC", "BluetoothMAC",
		}
		count := 0
		for _, d := range devices {
			payload := map[string]interface{}{
				"udid": d.UDID, "request_type": "DeviceInformation", "queries": queries,
			}
			if _, err := mdmClient.SendCommand(r.Context(), payload); err != nil {
				continue
			}
			_ = mdmClient.SendPush(r.Context(), d.UDID)
			count++
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"count": count})
		log.Printf("[sync-info] sent DeviceInformation to %d devices", count)
	})

	// Assets CRUD API
	mux.HandleFunc("/api/assets", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			// List assets, optionally filter by device_udid
			deviceUdid := r.URL.Query().Get("device_udid")
			q := `SELECT a.id, a.device_udid, a.asset_number, a.name, a.spec, a.quantity, a.unit,
			             a.acquired_date, a.unit_price, a.purpose, a.borrow_date,
			             a.custodian_id, a.custodian_name, a.location, a.asset_category, a.notes,
			             a.created_at, a.updated_at,
			             COALESCE(d.device_name,'') as device_name, COALESCE(d.serial_number,'') as device_serial,
			             a.category_id, COALESCE(c.name,'') as category_name,
			             COALESCE(a.asset_status,'available') as asset_status
			      FROM assets a LEFT JOIN devices d ON a.device_udid = d.udid
			      LEFT JOIN categories c ON a.category_id = c.id`
			args := []interface{}{}
			if deviceUdid != "" {
				q += ` WHERE a.device_udid = $1`
				args = append(args, deviceUdid)
			}
			q += ` ORDER BY a.created_at DESC`

			rows, err := pool.Query(r.Context(), q, args...)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			defer rows.Close()

			type assetRow struct {
				ID            string  `json:"id"`
				DeviceUdid    *string `json:"device_udid"`
				AssetNumber   string  `json:"asset_number"`
				Name          string  `json:"name"`
				Spec          string  `json:"spec"`
				Quantity      int     `json:"quantity"`
				Unit          string  `json:"unit"`
				AcquiredDate  *string `json:"acquired_date"`
				UnitPrice     float64 `json:"unit_price"`
				Purpose       string  `json:"purpose"`
				BorrowDate    *string `json:"borrow_date"`
				CustodianID   *string `json:"custodian_id"`
				CustodianName string  `json:"custodian_name"`
				Location      string  `json:"location"`
				AssetCategory string  `json:"asset_category"`
				Notes         string  `json:"notes"`
				CreatedAt     string  `json:"created_at"`
				UpdatedAt     string  `json:"updated_at"`
				DeviceName    string  `json:"device_name"`
				DeviceSerial  string  `json:"device_serial"`
				CategoryID    *string `json:"category_id"`
				CategoryName  string  `json:"category_name"`
				AssetStatus   string  `json:"asset_status"`
			}
			var assets []assetRow
			for rows.Next() {
				var a assetRow
				var acquiredDate, borrowDate *time.Time
				var custodianID *string
				var deviceUdidPtr *string
				var createdAt, updatedAt time.Time
				if err := rows.Scan(&a.ID, &deviceUdidPtr, &a.AssetNumber, &a.Name, &a.Spec, &a.Quantity, &a.Unit,
					&acquiredDate, &a.UnitPrice, &a.Purpose, &borrowDate,
					&custodianID, &a.CustodianName, &a.Location, &a.AssetCategory, &a.Notes,
					&createdAt, &updatedAt, &a.DeviceName, &a.DeviceSerial,
					&a.CategoryID, &a.CategoryName, &a.AssetStatus); err != nil {
					continue
				}
				if deviceUdidPtr != nil {
					a.DeviceUdid = deviceUdidPtr
				}
				if acquiredDate != nil {
					s := acquiredDate.Format("2006-01-02")
					a.AcquiredDate = &s
				}
				if borrowDate != nil {
					s := borrowDate.Format("2006-01-02")
					a.BorrowDate = &s
				}
				if custodianID != nil {
					a.CustodianID = custodianID
				}
				a.CreatedAt = createdAt.Format(time.RFC3339)
				a.UpdatedAt = updatedAt.Format(time.RFC3339)
				assets = append(assets, a)
			}
			if assets == nil {
				assets = []assetRow{}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"assets": assets})

		case http.MethodPost:
			var body struct {
				DeviceUdid    *string `json:"device_udid"`
				AssetNumber   string  `json:"asset_number"`
				Name          string  `json:"name"`
				Spec          string  `json:"spec"`
				Quantity      int     `json:"quantity"`
				Unit          string  `json:"unit"`
				AcquiredDate  *string `json:"acquired_date"`
				UnitPrice     float64 `json:"unit_price"`
				Purpose       string  `json:"purpose"`
				BorrowDate    *string `json:"borrow_date"`
				CustodianID   *string `json:"custodian_id"`
				CustodianName string  `json:"custodian_name"`
				Location      string  `json:"location"`
				AssetCategory string  `json:"asset_category"`
				Notes         string  `json:"notes"`
				CategoryID    *string `json:"category_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var id string
			err := pool.QueryRow(r.Context(),
				`INSERT INTO assets (device_udid, asset_number, name, spec, quantity, unit, acquired_date, unit_price, purpose, borrow_date, custodian_id, custodian_name, location, asset_category, notes, category_id)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`,
				body.DeviceUdid, body.AssetNumber, body.Name, body.Spec, body.Quantity, body.Unit,
				body.AcquiredDate, body.UnitPrice, body.Purpose, body.BorrowDate,
				body.CustodianID, body.CustodianName, body.Location, body.AssetCategory, body.Notes, body.CategoryID,
			).Scan(&id)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"id": id})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Asset by ID — PUT (update) / DELETE
	mux.HandleFunc("/api/assets/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/assets/")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodPut:
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			allowed := []string{"device_udid", "asset_number", "name", "spec", "quantity", "unit",
				"acquired_date", "unit_price", "purpose", "borrow_date",
				"custodian_id", "custodian_name", "location", "asset_category", "notes", "category_id", "asset_status"}
			sets := []string{}
			args := []interface{}{}
			idx := 1
			for _, k := range allowed {
				if v, ok := body[k]; ok {
					sets = append(sets, k+"=$"+fmt.Sprint(idx))
					args = append(args, v)
					idx++
				}
			}
			if len(sets) == 0 {
				w.Write([]byte(`{"ok":true}`))
				return
			}
			q := "UPDATE assets SET " + strings.Join(sets, ", ") + ", updated_at=now() WHERE id=$" + fmt.Sprint(idx)
			args = append(args, id)
			if _, err := pool.Exec(r.Context(), q, args...); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			w.Write([]byte(`{"ok":true}`))

		case http.MethodDelete:
			if _, err := pool.Exec(r.Context(), `DELETE FROM assets WHERE id=$1`, id); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{"ok":true}`))

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// iTunes Lookup API proxy — avoids CORS, returns app info including icon
	mux.HandleFunc("/api/itunes-lookup", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		bundleID := r.URL.Query().Get("bundleId")
		itunesID := r.URL.Query().Get("id")
		if bundleID == "" && itunesID == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"bundleId or id required"}`))
			return
		}
		var lookupURL string
		if bundleID != "" {
			lookupURL = "https://itunes.apple.com/lookup?bundleId=" + bundleID + "&country=tw"
		} else {
			lookupURL = "https://itunes.apple.com/lookup?id=" + itunesID + "&country=tw"
		}
		resp, err := http.Get(lookupURL)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, resp.Body)
	})

	// iTunes Search API proxy — search apps by keyword
	mux.HandleFunc("/api/itunes-search", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		term := r.URL.Query().Get("term")
		if term == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"term required"}`))
			return
		}
		limit := r.URL.Query().Get("limit")
		if limit == "" {
			limit = "10"
		}
		searchURL := fmt.Sprintf("https://itunes.apple.com/search?term=%s&country=tw&entity=software&limit=%s", term, limit)
		resp, err := http.Get(searchURL)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, resp.Body)
	})

	// Managed Apps CRUD — registry of apps available for installation
	mux.HandleFunc("/api/managed-apps", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			rows, err := pool.Query(r.Context(),
				`SELECT ma.id, ma.name, ma.bundle_id, ma.app_type, ma.itunes_store_id, ma.manifest_url,
				        ma.purchased_qty, ma.notes, ma.created_at, ma.updated_at,
				        (SELECT COUNT(*) FROM device_apps da WHERE da.app_id = ma.id) as installed_count,
				        ma.icon_url
				 FROM managed_apps ma ORDER BY ma.name`)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			defer rows.Close()

			type appRow struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				BundleID       string `json:"bundle_id"`
				AppType        string `json:"app_type"`
				ItunesStoreID  string `json:"itunes_store_id"`
				ManifestURL    string `json:"manifest_url"`
				PurchasedQty   int    `json:"purchased_qty"`
				Notes          string `json:"notes"`
				CreatedAt      string `json:"created_at"`
				UpdatedAt      string `json:"updated_at"`
				InstalledCount int    `json:"installed_count"`
				IconURL        string `json:"icon_url"`
			}
			var apps []appRow
			for rows.Next() {
				var a appRow
				var createdAt, updatedAt time.Time
				if err := rows.Scan(&a.ID, &a.Name, &a.BundleID, &a.AppType, &a.ItunesStoreID, &a.ManifestURL,
					&a.PurchasedQty, &a.Notes, &createdAt, &updatedAt, &a.InstalledCount, &a.IconURL); err != nil {
					continue
				}
				a.CreatedAt = createdAt.Format(time.RFC3339)
				a.UpdatedAt = updatedAt.Format(time.RFC3339)
				apps = append(apps, a)
			}
			if apps == nil {
				apps = []appRow{}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"apps": apps})

		case http.MethodPost:
			if claims.Role != "admin" && claims.Role != "operator" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			var body struct {
				Name          string `json:"name"`
				BundleID      string `json:"bundle_id"`
				AppType       string `json:"app_type"`
				ItunesStoreID string `json:"itunes_store_id"`
				ManifestURL   string `json:"manifest_url"`
				PurchasedQty  int    `json:"purchased_qty"`
				Notes         string `json:"notes"`
				IconURL       string `json:"icon_url"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"name is required"}`))
				return
			}
			if body.AppType == "" {
				body.AppType = "vpp"
			}
			var id string
			err := pool.QueryRow(r.Context(),
				`INSERT INTO managed_apps (name, bundle_id, app_type, itunes_store_id, manifest_url, purchased_qty, notes, icon_url)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
				body.Name, body.BundleID, body.AppType, body.ItunesStoreID, body.ManifestURL, body.PurchasedQty, body.Notes, body.IconURL,
			).Scan(&id)
			if err != nil {
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"id": id})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Managed App by ID — PUT / DELETE
	mux.HandleFunc("/api/managed-apps/", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if claims.Role != "admin" && claims.Role != "operator" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/managed-apps/")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodPut:
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			allowed := []string{"name", "bundle_id", "app_type", "itunes_store_id", "manifest_url", "purchased_qty", "notes", "icon_url"}
			sets := []string{}
			args := []interface{}{}
			idx := 1
			for _, k := range allowed {
				if v, ok := body[k]; ok {
					sets = append(sets, fmt.Sprintf("%s=$%d", k, idx))
					args = append(args, v)
					idx++
				}
			}
			if len(sets) == 0 {
				w.Write([]byte(`{"ok":true}`))
				return
			}
			q := fmt.Sprintf("UPDATE managed_apps SET %s, updated_at=now() WHERE id=$%d", strings.Join(sets, ", "), idx)
			args = append(args, id)
			if _, err := pool.Exec(r.Context(), q, args...); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			w.Write([]byte(`{"ok":true}`))

		case http.MethodDelete:
			if _, err := pool.Exec(r.Context(), `DELETE FROM managed_apps WHERE id=$1`, id); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{"ok":true}`))

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Device Apps — list apps installed on a device
	mux.HandleFunc("/api/device-apps", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			deviceUdid := r.URL.Query().Get("device_udid")
			if deviceUdid == "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"device_udid required"}`))
				return
			}
			rows, err := pool.Query(r.Context(),
				`SELECT da.id, da.device_udid, da.app_id, da.installed_at,
				        ma.name, ma.bundle_id, ma.app_type
				 FROM device_apps da JOIN managed_apps ma ON da.app_id = ma.id
				 WHERE da.device_udid = $1 ORDER BY da.installed_at DESC`, deviceUdid)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			type row struct {
				ID          string `json:"id"`
				DeviceUdid  string `json:"device_udid"`
				AppID       string `json:"app_id"`
				InstalledAt string `json:"installed_at"`
				AppName     string `json:"app_name"`
				BundleID    string `json:"bundle_id"`
				AppType     string `json:"app_type"`
			}
			var items []row
			for rows.Next() {
				var r row
				var installedAt time.Time
				if err := rows.Scan(&r.ID, &r.DeviceUdid, &r.AppID, &installedAt, &r.AppName, &r.BundleID, &r.AppType); err != nil {
					continue
				}
				r.InstalledAt = installedAt.Format(time.RFC3339)
				items = append(items, r)
			}
			if items == nil {
				items = []row{}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"device_apps": items})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Install app to device — creates binding + sends MDM command
	mux.HandleFunc("/api/device-apps/install", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if claims.Role != "admin" && claims.Role != "operator" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var body struct {
			AppID string `json:"app_id"`
			UDID  string `json:"udid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AppID == "" || body.UDID == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"app_id and udid required"}`))
			return
		}

		// Look up the managed app
		var appType, itunesStoreID, manifestURL, bundleID, appName string
		var purchasedQty int
		err = pool.QueryRow(r.Context(),
			`SELECT app_type, itunes_store_id, manifest_url, bundle_id, name, purchased_qty FROM managed_apps WHERE id=$1`, body.AppID,
		).Scan(&appType, &itunesStoreID, &manifestURL, &bundleID, &appName, &purchasedQty)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"app not found"}`))
			return
		}

		// Check available quantity
		var installedCount int
		pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM device_apps WHERE app_id=$1`, body.AppID).Scan(&installedCount)
		if purchasedQty > 0 && installedCount >= purchasedQty {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("已達採購上限 (%d/%d)", installedCount, purchasedQty)})
			return
		}

		// Check if already installed on this device
		var exists int
		pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM device_apps WHERE device_udid=$1 AND app_id=$2`, body.UDID, body.AppID).Scan(&exists)
		if exists > 0 {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error":"此 App 已安裝在該裝置上"}`))
			return
		}

		// Send MDM command
		var payload map[string]interface{}
		if appType == "enterprise" {
			payload = map[string]interface{}{
				"udid":         body.UDID,
				"request_type": "InstallEnterpriseApplication",
				"manifest_url": manifestURL,
			}
		} else {
			// VPP app — assign license first if VPP client available
			if vppClient != nil && itunesStoreID != "" {
				// Get device serial number for VPP
				dev, devErr := deviceRepo.GetByUDID(r.Context(), body.UDID)
				if devErr == nil && dev.SerialNumber != "" {
					_, _ = vppClient.AssignLicense(r.Context(), itunesStoreID, []string{dev.SerialNumber})
				}
			}
			payload = map[string]interface{}{
				"udid":            body.UDID,
				"request_type":    "InstallApplication",
				"itunes_store_id": itunesStoreID,
				"options":         map[string]interface{}{"purchase_method": 1},
			}
		}

		result, cmdErr := mdmClient.SendCommand(r.Context(), payload)
		if cmdErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": cmdErr.Error()})
			return
		}
		_ = mdmClient.SendPush(r.Context(), body.UDID)

		// Create device_app binding
		pool.Exec(r.Context(),
			`INSERT INTO device_apps (device_udid, app_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			body.UDID, body.AppID)

		// Audit log
		_ = auditRepo.Create(r.Context(), &domain.AuditLog{
			UserID: claims.UserID, Username: claims.Username,
			Action: "install_app", Target: body.UDID, Detail: appName + " (" + bundleID + ")",
		})

		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":           true,
			"command_uuid": result.CommandUUID,
			"raw_response": result.RawResponse,
		})
	})

	// Update app on device — re-sends install command to trigger update (all authenticated users)
	mux.HandleFunc("/api/device-apps/update", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var body struct {
			AppID string `json:"app_id"`
			UDID  string `json:"udid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AppID == "" || body.UDID == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"app_id and udid required"}`))
			return
		}

		var appType, itunesStoreID, manifestURL, bundleID, appName string
		err = pool.QueryRow(r.Context(),
			`SELECT app_type, itunes_store_id, manifest_url, bundle_id, name FROM managed_apps WHERE id=$1`, body.AppID,
		).Scan(&appType, &itunesStoreID, &manifestURL, &bundleID, &appName)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"app not found"}`))
			return
		}

		var payload map[string]interface{}
		if appType == "enterprise" {
			payload = map[string]interface{}{
				"udid":         body.UDID,
				"request_type": "InstallEnterpriseApplication",
				"manifest_url": manifestURL,
			}
		} else {
			payload = map[string]interface{}{
				"udid":            body.UDID,
				"request_type":    "InstallApplication",
				"itunes_store_id": itunesStoreID,
				"options":         map[string]interface{}{"purchase_method": 1},
			}
		}

		result, cmdErr := mdmClient.SendCommand(r.Context(), payload)
		if cmdErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": cmdErr.Error()})
			return
		}
		_ = mdmClient.SendPush(r.Context(), body.UDID)

		_ = auditRepo.Create(r.Context(), &domain.AuditLog{
			UserID: claims.UserID, Username: claims.Username,
			Action: "update_app", Target: body.UDID, Detail: appName + " (" + bundleID + ")",
		})

		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":           true,
			"command_uuid": result.CommandUUID,
			"raw_response": result.RawResponse,
		})
	})

	// Uninstall app from device — removes binding + sends MDM command
	mux.HandleFunc("/api/device-apps/uninstall", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if claims.Role != "admin" && claims.Role != "operator" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var body struct {
			AppID string `json:"app_id"`
			UDID  string `json:"udid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AppID == "" || body.UDID == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"app_id and udid required"}`))
			return
		}

		// Look up bundle_id for the remove command
		var bundleID, appName string
		err = pool.QueryRow(r.Context(),
			`SELECT bundle_id, name FROM managed_apps WHERE id=$1`, body.AppID,
		).Scan(&bundleID, &appName)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"app not found"}`))
			return
		}

		// Send MDM remove command
		result, cmdErr := mdmClient.SendCommand(r.Context(), map[string]interface{}{
			"udid":         body.UDID,
			"request_type": "RemoveApplication",
			"identifier":   bundleID,
		})
		if cmdErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": cmdErr.Error()})
			return
		}
		_ = mdmClient.SendPush(r.Context(), body.UDID)

		// Remove binding
		pool.Exec(r.Context(), `DELETE FROM device_apps WHERE device_udid=$1 AND app_id=$2`, body.UDID, body.AppID)

		// Audit log
		_ = auditRepo.Create(r.Context(), &domain.AuditLog{
			UserID: claims.UserID, Username: claims.Username,
			Action: "remove_app", Target: body.UDID, Detail: appName + " (" + bundleID + ")",
		})

		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":           true,
			"command_uuid": result.CommandUUID,
			"raw_response": result.RawResponse,
		})
	})

	// Sync installed apps — match device installed_apps_raw against managed_apps by bundle_id
	mux.HandleFunc("/api/sync-device-apps", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		// 1. Load all managed apps bundle_id → id
		maRows, err := pool.Query(r.Context(), `SELECT id, bundle_id FROM managed_apps`)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		defer maRows.Close()
		bundleMap := map[string]string{} // bundle_id → managed_app id
		for maRows.Next() {
			var id, bid string
			if err := maRows.Scan(&id, &bid); err == nil && bid != "" {
				bundleMap[bid] = id
			}
		}
		if len(bundleMap) == 0 {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "synced": 0, "message": "no managed apps"})
			return
		}

		// 2. Load all devices that have installed_apps_raw
		devRows, err := pool.Query(r.Context(), `SELECT udid, details->'installed_apps_raw' FROM devices WHERE details ? 'installed_apps_raw'`)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		defer devRows.Close()

		synced := 0
		for devRows.Next() {
			var udid, rawB64 string
			if err := devRows.Scan(&udid, &rawB64); err != nil {
				continue
			}
			// rawB64 may be JSON-quoted string
			rawB64 = strings.Trim(rawB64, `"`)
			decoded, err := base64.StdEncoding.DecodeString(rawB64)
			if err != nil {
				continue
			}
			xmlStr := string(decoded)

			// 3. Extract all Identifier values from InstalledApplicationList
			remaining := xmlStr
			for {
				keyTag := "<key>Identifier</key>"
				pos := strings.Index(remaining, keyTag)
				if pos < 0 {
					break
				}
				after := remaining[pos+len(keyTag):]
				sStart := strings.Index(after, "<string>")
				sEnd := strings.Index(after, "</string>")
				if sStart < 0 || sEnd <= sStart {
					remaining = after
					continue
				}
				bundleID := after[sStart+8 : sEnd]
				remaining = after[sEnd:]

				// 4. If this bundle_id matches a managed app, create binding
				if appID, ok := bundleMap[bundleID]; ok {
					tag, err := pool.Exec(r.Context(),
						`INSERT INTO device_apps (device_udid, app_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
						udid, appID)
					if err == nil && tag.RowsAffected() > 0 {
						synced++
					}
				}
			}
		}

		_ = auditRepo.Create(r.Context(), &domain.AuditLog{
			UserID: claims.UserID, Username: claims.Username,
			Action: "sync_device_apps", Target: "", Detail: fmt.Sprintf("synced %d bindings", synced),
		})

		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "synced": synced})
	})

	// User update API
	mux.HandleFunc("/api/users/", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil || claims.Role != "admin" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/users/")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodPut {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			sets := []string{}
			args := []interface{}{}
			idx := 1
			for _, k := range []string{"role", "display_name", "is_active"} {
				if v, ok := body[k]; ok {
					sets = append(sets, fmt.Sprintf("%s=$%d", k, idx))
					args = append(args, v)
					idx++
				}
			}
			// Password update
			if pw, ok := body["password"].(string); ok && pw != "" {
				hash, err := service.HashArgon2id(pw)
				if err == nil {
					sets = append(sets, fmt.Sprintf("password_hash=$%d", idx))
					args = append(args, hash)
					idx++
				}
			}
			if len(sets) == 0 {
				w.Write([]byte(`{"ok":true}`))
				return
			}
			q := fmt.Sprintf("UPDATE users SET %s, updated_at=now() WHERE id=$%d", strings.Join(sets, ", "), idx)
			args = append(args, id)
			if _, err := pool.Exec(r.Context(), q, args...); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{"ok":true}`))
			return
		}
		if r.Method == http.MethodDelete {
			pool.Exec(r.Context(), `DELETE FROM users WHERE id=$1`, id)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true}`))
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// Users list API (for dropdowns — returns id, username, display_name)
	mux.HandleFunc("/api/users-list", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		rows, err := pool.Query(r.Context(), `SELECT id, username, display_name, role, is_active FROM users ORDER BY username`)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type u struct {
			ID          string `json:"id"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Role        string `json:"role"`
			IsActive    bool   `json:"is_active"`
		}
		var users []u
		for rows.Next() {
			var item u
			rows.Scan(&item.ID, &item.Username, &item.DisplayName, &item.Role, &item.IsActive)
			users = append(users, item)
		}
		if users == nil {
			users = []u{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"users": users})
	})

	// Rentals API
	mux.HandleFunc("/api/rentals", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			statusFilter := r.URL.Query().Get("status")
			deviceUdid := r.URL.Query().Get("device_udid")
			showArchived := r.URL.Query().Get("show_archived")
			q := `SELECT r.id, r.device_udid, r.borrower_id, r.borrower_name, r.approver_id, r.approver_name,
			             r.status, r.purpose, r.borrow_date, r.expected_return, r.actual_return, r.notes,
			             r.created_at, r.updated_at,
			             COALESCE(d.device_name,'') as device_name, COALESCE(d.serial_number,'') as device_serial,
			             a.custodian_id, COALESCE(a.custodian_name,'') as custodian_name,
			             r.rental_number, r.is_archived, r.return_checklist, r.return_notes
			      FROM rentals r LEFT JOIN devices d ON r.device_udid = d.udid
			      LEFT JOIN assets a ON a.device_udid = r.device_udid WHERE 1=1`
			args := []interface{}{}
			idx := 1
			if statusFilter != "" {
				q += fmt.Sprintf(` AND r.status=$%d`, idx)
				args = append(args, statusFilter)
				idx++
			}
			if deviceUdid != "" {
				q += fmt.Sprintf(` AND r.device_udid=$%d`, idx)
				args = append(args, deviceUdid)
				idx++
			}
			if showArchived != "true" {
				q += ` AND r.is_archived = false`
			}
			q += ` ORDER BY r.rental_number DESC`

			rows, err := pool.Query(r.Context(), q, args...)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			type rentalRow struct {
				ID              string                 `json:"id"`
				DeviceUdid      string                 `json:"device_udid"`
				BorrowerID      string                 `json:"borrower_id"`
				BorrowerName    string                 `json:"borrower_name"`
				ApproverID      *string                `json:"approver_id"`
				ApproverName    string                 `json:"approver_name"`
				Status          string                 `json:"status"`
				Purpose         string                 `json:"purpose"`
				BorrowDate      string                 `json:"borrow_date"`
				ExpectedReturn  *string                `json:"expected_return"`
				ActualReturn    *string                `json:"actual_return"`
				Notes           string                 `json:"notes"`
				CreatedAt       string                 `json:"created_at"`
				UpdatedAt       string                 `json:"updated_at"`
				DeviceName      string                 `json:"device_name"`
				DeviceSerial    string                 `json:"device_serial"`
				CustodianID     *string                `json:"custodian_id"`
				CustodianName   string                 `json:"custodian_name"`
				RentalNumber    int                    `json:"rental_number"`
				IsArchived      bool                   `json:"is_archived"`
				ReturnChecklist map[string]interface{} `json:"return_checklist"`
				ReturnNotes     string                 `json:"return_notes"`
			}
			var rentals []rentalRow
			for rows.Next() {
				var r2 rentalRow
				var borrowDate, createdAt, updatedAt time.Time
				var expectedReturn *time.Time
				var actualReturn *time.Time
				var approverID *string
				var checklistJSON []byte
				if err := rows.Scan(&r2.ID, &r2.DeviceUdid, &r2.BorrowerID, &r2.BorrowerName, &approverID, &r2.ApproverName,
					&r2.Status, &r2.Purpose, &borrowDate, &expectedReturn, &actualReturn, &r2.Notes,
					&createdAt, &updatedAt, &r2.DeviceName, &r2.DeviceSerial,
					&r2.CustodianID, &r2.CustodianName, &r2.RentalNumber, &r2.IsArchived,
					&checklistJSON, &r2.ReturnNotes); err != nil {
					log.Printf("rental scan: %v", err)
					continue
				}
				r2.BorrowDate = borrowDate.Format(time.RFC3339)
				r2.CreatedAt = createdAt.Format(time.RFC3339)
				r2.UpdatedAt = updatedAt.Format(time.RFC3339)
				if approverID != nil {
					r2.ApproverID = approverID
				}
				if len(checklistJSON) > 0 {
					json.Unmarshal(checklistJSON, &r2.ReturnChecklist)
				}
				if expectedReturn != nil {
					s := expectedReturn.Format("2006-01-02")
					r2.ExpectedReturn = &s
				}
				if actualReturn != nil {
					s := actualReturn.Format(time.RFC3339)
					r2.ActualReturn = &s
				}
				rentals = append(rentals, r2)
			}
			if rentals == nil {
				rentals = []rentalRow{}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"rentals": rentals})

		case http.MethodPost:
			var body struct {
				DeviceUdids    []string `json:"device_udids"`
				BorrowerID     string   `json:"borrower_id"`
				Purpose        string   `json:"purpose"`
				ExpectedReturn *string  `json:"expected_return"`
				Notes          string   `json:"notes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.BorrowerID == "" || len(body.DeviceUdids) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"device_udids and borrower_id required"}`))
				return
			}
			// Get borrower name
			var borrowerName string
			pool.QueryRow(r.Context(), `SELECT COALESCE(display_name, username) FROM users WHERE id=$1`, body.BorrowerID).Scan(&borrowerName)

			// Check all devices are available (not rented, broken, repairing, lost, retired)
			var unavailable []string
			for _, udid := range body.DeviceUdids {
				var assetStatus string
				var isRented bool
				var isLostMode bool
				pool.QueryRow(r.Context(),
					`SELECT COALESCE(a.asset_status,'available'),
					        EXISTS(SELECT 1 FROM rentals rl WHERE rl.device_udid=$1 AND rl.status IN ('pending','approved','active')),
					        d.is_lost_mode
					 FROM devices d LEFT JOIN assets a ON a.device_udid=d.udid WHERE d.udid=$1`, udid,
				).Scan(&assetStatus, &isRented, &isLostMode)
				if isRented {
					unavailable = append(unavailable, udid+" (借出中)")
				} else if isLostMode {
					unavailable = append(unavailable, udid+" (遺失)")
				} else if assetStatus != "available" && assetStatus != "" {
					unavailable = append(unavailable, udid+" ("+assetStatus+")")
				}
			}
			if len(unavailable) > 0 {
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "部分裝置目前無法借出",
					"devices": unavailable,
				})
				return
			}

			// Get next rental_number for the batch (all devices share the same number)
			var rentalNumber int
			pool.QueryRow(r.Context(), `SELECT COALESCE(MAX(rental_number), 0) + 1 FROM rentals`).Scan(&rentalNumber)

			var ids []string
			for _, udid := range body.DeviceUdids {
				var id string
				err := pool.QueryRow(r.Context(),
					`INSERT INTO rentals (device_udid, borrower_id, borrower_name, purpose, expected_return, notes, rental_number)
					 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
					udid, body.BorrowerID, borrowerName, body.Purpose, body.ExpectedReturn, body.Notes, rentalNumber,
				).Scan(&id)
				if err != nil {
					log.Printf("rental insert: %v", err)
					continue
				}
				ids = append(ids, id)
			}
			_ = claims // used for auth check
			json.NewEncoder(w).Encode(map[string]interface{}{"ids": ids, "count": len(ids), "rental_number": rentalNumber})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Rental actions — approve, activate, return, reject
	mux.HandleFunc("/api/rentals/", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/rentals/"), "/")
		id := parts[0]
		action := ""
		if len(parts) > 1 {
			action = parts[1]
		}

		if r.Method == http.MethodPost && action != "" {
			// Get approver display name
			var approverDisplayName string
			pool.QueryRow(r.Context(), `SELECT COALESCE(display_name, username) FROM users WHERE id=$1`, claims.UserID).Scan(&approverDisplayName)

			// Get rental info — and its rental_number to apply batch actions
			var deviceUdid, status string
			var rentalNumber int
			if err := pool.QueryRow(r.Context(), `SELECT device_udid, status, rental_number FROM rentals WHERE id=$1`, id).Scan(&deviceUdid, &status, &rentalNumber); err != nil {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"rental not found"}`))
				return
			}

			switch action {
			case "approve":
				if status != "pending" {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error":"rental is not pending"}`))
					return
				}
				pool.Exec(r.Context(),
					`UPDATE rentals SET status='approved', approver_id=$1, approver_name=$2, updated_at=now() WHERE rental_number=$3 AND status='pending'`,
					claims.UserID, approverDisplayName, rentalNumber)
				w.Write([]byte(`{"ok":true,"status":"approved"}`))

			case "activate":
				if status != "approved" {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error":"rental is not approved"}`))
					return
				}
				// Activate all in batch
				pool.Exec(r.Context(), `UPDATE rentals SET status='active', borrow_date=now(), updated_at=now() WHERE rental_number=$1 AND status='approved'`, rentalNumber)
				// Update asset custodian for all devices in batch
				var borrowerID, borrowerName string
				pool.QueryRow(r.Context(), `SELECT borrower_id, borrower_name FROM rentals WHERE id=$1`, id).Scan(&borrowerID, &borrowerName)
				batchRows, _ := pool.Query(r.Context(), `SELECT device_udid FROM rentals WHERE rental_number=$1`, rentalNumber)
				if batchRows != nil {
					for batchRows.Next() {
						var batchUdid string
						batchRows.Scan(&batchUdid)
						pool.Exec(r.Context(), `UPDATE assets SET custodian_id=$1, custodian_name=$2, borrow_date=now() WHERE device_udid=$3`,
							borrowerID, borrowerName, batchUdid)
					}
					batchRows.Close()
				}

				log.Printf("[rental] batch activated: rental_number=%d borrower=%s", rentalNumber, borrowerName)
				w.Write([]byte(`{"ok":true,"status":"active"}`))

			case "return":
				if status != "active" {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error":"rental is not active"}`))
					return
				}
				// Read return checklist + notes from request body
				var returnBody struct {
					Notes     string                 `json:"notes"`
					Checklist map[string]interface{} `json:"checklist"`
				}
				json.NewDecoder(r.Body).Decode(&returnBody)

				var checklistJSON []byte
				if returnBody.Checklist != nil {
					checklistJSON, _ = json.Marshal(returnBody.Checklist)
				}

				// Return all in batch with checklist data
				pool.Exec(r.Context(),
					`UPDATE rentals SET status='returned', actual_return=now(), updated_at=now(),
					 return_checklist=$1, return_notes=$2
					 WHERE rental_number=$3 AND status='active'`,
					checklistJSON, returnBody.Notes, rentalNumber)
				// Restore custodian for all devices in batch
				batchRows2, _ := pool.Query(r.Context(), `SELECT device_udid FROM rentals WHERE rental_number=$1`, rentalNumber)
				if batchRows2 != nil {
					for batchRows2.Next() {
						var batchUdid string
						batchRows2.Scan(&batchUdid)
						pool.Exec(r.Context(),
							`UPDATE assets SET custodian_id=$1, custodian_name=$2, borrow_date=NULL WHERE device_udid=$3`,
							claims.UserID, approverDisplayName, batchUdid)
					}
					batchRows2.Close()
				}

				log.Printf("[rental] batch returned: rental_number=%d custodian restored to %s", rentalNumber, approverDisplayName)
				w.Write([]byte(`{"ok":true,"status":"returned"}`))

			case "reject":
				if status != "pending" {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error":"rental is not pending"}`))
					return
				}
				pool.Exec(r.Context(),
					`UPDATE rentals SET status='rejected', approver_id=$1, approver_name=$2, updated_at=now() WHERE rental_number=$3 AND status='pending'`,
					claims.UserID, approverDisplayName, rentalNumber)
				w.Write([]byte(`{"ok":true,"status":"rejected"}`))

			default:
				w.WriteHeader(http.StatusBadRequest)
			}
			return
		}

		if r.Method == http.MethodDelete {
			var rentalNumber int
			pool.QueryRow(r.Context(), `SELECT rental_number FROM rentals WHERE id=$1`, id).Scan(&rentalNumber)
			pool.Exec(r.Context(), `DELETE FROM rentals WHERE rental_number=$1`, rentalNumber)
			w.Write([]byte(`{"ok":true}`))
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// Batch archive rentals
	mux.HandleFunc("/api/rentals-archive", func(w http.ResponseWriter, r *http.Request) {
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if claims.Role != "admin" && claims.Role != "operator" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var body struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"ids required"}`))
			return
		}

		placeholders := make([]string, len(body.IDs))
		args := []interface{}{}
		for i, id := range body.IDs {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args = append(args, id)
		}
		q := fmt.Sprintf("UPDATE rentals SET is_archived = true, updated_at = now() WHERE id IN (%s)", strings.Join(placeholders, ","))
		_, err = pool.Exec(r.Context(), q, args...)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Write([]byte(`{"ok":true}`))
	})

	// Categories API (tree structure)
	mux.HandleFunc("/api/categories", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			rows, err := pool.Query(r.Context(),
				`SELECT id, parent_id, name, level, sort_order FROM categories ORDER BY sort_order, name`)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			type cat struct {
				ID        string  `json:"id"`
				ParentID  *string `json:"parent_id"`
				Name      string  `json:"name"`
				Level     int     `json:"level"`
				SortOrder int     `json:"sort_order"`
			}
			var allCats []cat
			for rows.Next() {
				var c cat
				rows.Scan(&c.ID, &c.ParentID, &c.Name, &c.Level, &c.SortOrder)
				allCats = append(allCats, c)
			}
			// Build tree-ordered flat list: parent then children recursively
			var sorted []cat
			var walk func(parentID *string)
			walk = func(parentID *string) {
				for _, c := range allCats {
					match := false
					if parentID == nil && c.ParentID == nil {
						match = true
					} else if parentID == nil && c.ParentID != nil && *c.ParentID == "" {
						match = true
					} else if parentID != nil && c.ParentID != nil && *c.ParentID == *parentID {
						match = true
					}
					if match {
						sorted = append(sorted, c)
						walk(&c.ID)
					}
				}
			}
			walk(nil)
			if sorted == nil {
				sorted = []cat{}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"categories": sorted})

		case http.MethodPost:
			var body struct {
				ParentID *string `json:"parent_id"`
				Name     string  `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			level := 0
			if body.ParentID != nil && *body.ParentID != "" {
				pool.QueryRow(r.Context(), `SELECT level FROM categories WHERE id=$1`, *body.ParentID).Scan(&level)
				level++
			}
			var id string
			pool.QueryRow(r.Context(),
				`INSERT INTO categories (parent_id, name, level) VALUES ($1, $2, $3) RETURNING id`,
				body.ParentID, body.Name, level).Scan(&id)
			json.NewEncoder(w).Encode(map[string]string{"id": id})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/categories/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/categories/")
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodPut:
			var body struct {
				Name string `json:"name"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			pool.Exec(r.Context(), `UPDATE categories SET name=$1 WHERE id=$2`, body.Name, id)
			w.Write([]byte(`{"ok":true}`))
		case http.MethodDelete:
			pool.Exec(r.Context(), `DELETE FROM categories WHERE id=$1`, id)
			w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Profile management API (requires JWT auth)
	mux.HandleFunc("/api/profiles", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Auth check
		claims, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		switch r.Method {
		case http.MethodGet:
			// List profiles
			rows, err := pool.Query(r.Context(),
				`SELECT id, name, filename, size, uploaded_by, created_at FROM profiles ORDER BY created_at DESC`)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"query failed"}`))
				return
			}
			defer rows.Close()

			type profileItem struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				Filename   string `json:"filename"`
				Size       int    `json:"size"`
				UploadedBy string `json:"uploaded_by"`
				CreatedAt  string `json:"created_at"`
			}
			var profiles []profileItem
			for rows.Next() {
				var p profileItem
				var createdAt time.Time
				if err := rows.Scan(&p.ID, &p.Name, &p.Filename, &p.Size, &p.UploadedBy, &createdAt); err != nil {
					log.Printf("profile scan: %v", err)
					continue
				}
				p.CreatedAt = createdAt.Format(time.RFC3339)
				profiles = append(profiles, p)
			}
			if profiles == nil {
				profiles = []profileItem{}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"profiles": profiles})

		case http.MethodPost:
			// Upload profile
			if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"file too large or invalid form"}`))
				return
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"file required"}`))
				return
			}
			defer file.Close()

			content, err := io.ReadAll(file)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"read failed"}`))
				return
			}

			name := r.FormValue("name")
			if name == "" {
				name = strings.TrimSuffix(header.Filename, ".mobileconfig")
			}

			var id string
			err = pool.QueryRow(r.Context(),
				`INSERT INTO profiles (name, filename, content, size, uploaded_by) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
				name, header.Filename, content, len(content), claims.Username).Scan(&id)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"insert failed"}`))
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "name": name, "filename": header.Filename, "size": len(content)})
			log.Printf("profile uploaded: %s (%s) by %s", name, header.Filename, claims.Username)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Profile download/delete by ID
	mux.HandleFunc("/api/profiles/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := middleware.ExtractTokenFromRequest(r, cfg.JWTSecret); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/profiles/")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			// Download profile content (base64 encoded for command use)
			var content []byte
			var filename string
			err := pool.QueryRow(r.Context(), `SELECT content, filename FROM profiles WHERE id=$1`, id).Scan(&content, &filename)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"not found"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			encoded := base64.StdEncoding.EncodeToString(content)
			json.NewEncoder(w).Encode(map[string]interface{}{"content_base64": encoded, "filename": filename})

		case http.MethodDelete:
			_, err := pool.Exec(r.Context(), `DELETE FROM profiles WHERE id=$1`, id)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"delete failed"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true}`))

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// CORS
	allowedOrigins := []string{"http://localhost:5173", "http://localhost:3000"}
	if env := os.Getenv("CORS_ORIGINS"); env != "" {
		allowedOrigins = strings.Split(env, ",")
	}
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Connect-Protocol-Version"},
		AllowCredentials: true,
	}).Handler(h2c.NewHandler(mux, &http2.Server{}))

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: corsHandler,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		srv.Shutdown(context.Background())
	}()

	log.Printf("MDM server listening on %s", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

func runMigrations(pool *pgxpool.Pool) {
	ctx := context.Background()
	for i, sql := range []string{db.MigrationSQL, db.Migration002SQL, db.Migration003SQL, db.Migration004SQL, db.Migration005SQL, db.Migration006SQL, db.Migration007SQL, db.Migration008SQL, db.Migration009SQL, db.Migration010SQL, db.Migration011SQL, db.Migration012SQL} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			log.Printf("migration %d: %v (may already be applied)", i+1, err)
		} else {
			log.Printf("migration %d: applied", i+1)
		}
	}
}

// backfillAppIcons queries iTunes Lookup API for VPP apps missing icon_url and fills them in.
func backfillAppIcons(pool *pgxpool.Pool) {
	ctx := context.Background()
	rows, err := pool.Query(ctx,
		`SELECT id, bundle_id, itunes_store_id FROM managed_apps WHERE icon_url = '' AND app_type = 'vpp' AND bundle_id != ''`)
	if err != nil {
		log.Printf("backfill icons query: %v", err)
		return
	}
	defer rows.Close()

	type appInfo struct{ id, bundleID, itunesID string }
	var apps []appInfo
	for rows.Next() {
		var a appInfo
		rows.Scan(&a.id, &a.bundleID, &a.itunesID)
		apps = append(apps, a)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	for _, a := range apps {
		lookupURL := fmt.Sprintf("https://itunes.apple.com/lookup?bundleId=%s&country=tw", a.bundleID)
		resp, err := client.Get(lookupURL)
		if err != nil {
			log.Printf("backfill icon %s: %v", a.bundleID, err)
			continue
		}
		var result struct {
			ResultCount int `json:"resultCount"`
			Results     []struct {
				ArtworkUrl512 string `json:"artworkUrl512"`
				ArtworkUrl100 string `json:"artworkUrl100"`
				TrackID       int    `json:"trackId"`
			} `json:"results"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if result.ResultCount == 0 {
			continue
		}
		r := result.Results[0]
		iconURL := r.ArtworkUrl512
		if iconURL == "" {
			iconURL = r.ArtworkUrl100
		}
		if iconURL == "" {
			continue
		}
		// Update icon_url (and itunes_store_id if missing)
		if a.itunesID == "" && r.TrackID > 0 {
			pool.Exec(ctx, `UPDATE managed_apps SET icon_url=$1, itunes_store_id=$2, updated_at=now() WHERE id=$3`,
				iconURL, fmt.Sprint(r.TrackID), a.id)
		} else {
			pool.Exec(ctx, `UPDATE managed_apps SET icon_url=$1, updated_at=now() WHERE id=$2`, iconURL, a.id)
		}
		log.Printf("backfill icon: %s → %s", a.bundleID, iconURL[:60]+"...")
	}
}
