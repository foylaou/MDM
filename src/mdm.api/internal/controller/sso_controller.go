package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/mdm-server/internal/config"
	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

const ssoStateCookie = "sso_state"
const ssoSecretPlaceholder = "********"

type SSOController struct {
	cfg          *config.Config
	userRepo     port.UserRepository
	settingsRepo port.SSOSettingsRepository
	auth         *middleware.AuthHelper

	// OIDC discovery document cache — invalidated when settings change.
	mu       sync.RWMutex
	oidcMeta *oidcDiscovery
	// issuerURL used when oidcMeta was last fetched; stale if issuer changed.
	cachedIssuer string
}

type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

func NewSSOController(cfg *config.Config, userRepo port.UserRepository, settingsRepo port.SSOSettingsRepository, auth *middleware.AuthHelper) *SSOController {
	return &SSOController{cfg: cfg, userRepo: userRepo, settingsRepo: settingsRepo, auth: auth}
}

func (c *SSOController) RegisterRoutes(mux *http.ServeMux) {
	// Auth flow — always registered; status endpoint tells UI if SSO is active.
	mux.HandleFunc("/api/auth/sso", c.handleStart)
	mux.HandleFunc("/api/auth/sso/callback", c.handleCallback)
	mux.HandleFunc("/api/auth/sso/status", c.handleStatus)

	// Admin settings — GET/PUT OIDC configuration stored in DB.
	mux.HandleFunc("/api/settings/sso", c.handleSettings)
}

// activeSettings returns DB settings if configured, falling back to env vars.
func (c *SSOController) activeSettings(ctx context.Context) *domain.SSOSettings {
	if c.settingsRepo != nil {
		if s, err := c.settingsRepo.Get(ctx); err == nil && s.ClientID != "" {
			return s
		}
	}
	// Env-var fallback
	return &domain.SSOSettings{
		Enabled:      c.cfg.OIDCEnabled,
		IssuerURL:    c.cfg.OIDCIssuerURL,
		ClientID:     c.cfg.OIDCClientID,
		ClientSecret: c.cfg.OIDCClientSecret,
		RedirectURL:  c.cfg.OIDCRedirectURL,
	}
}

// handleStatus tells the frontend whether SSO is currently enabled.
func (c *SSOController) handleStatus(w http.ResponseWriter, r *http.Request) {
	s := c.activeSettings(r.Context())
	writeJSON(w, map[string]bool{"enabled": s.Enabled && s.ClientID != ""})
}

// handleSettings is GET/PUT /api/settings/sso — requires sys_admin.
func (c *SSOController) handleSettings(w http.ResponseWriter, r *http.Request) {
	claims, err := c.auth.RequireSysAdmin(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s := c.activeSettings(r.Context())
		hasSecret := s.ClientSecret != ""
		writeJSON(w, map[string]interface{}{
			"enabled":           s.Enabled,
			"issuer_url":        s.IssuerURL,
			"client_id":         s.ClientID,
			"client_secret":     "",
			"has_client_secret": hasSecret,
			"redirect_url":      s.RedirectURL,
			"updated_at":        s.UpdatedAt.Format(time.RFC3339),
		})

	case http.MethodPut:
		var body struct {
			Enabled      bool   `json:"enabled"`
			IssuerURL    string `json:"issuer_url"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			RedirectURL  string `json:"redirect_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}

		// Preserve existing secret if placeholder sent.
		existing := c.activeSettings(r.Context())
		if body.ClientSecret == "" || body.ClientSecret == ssoSecretPlaceholder {
			body.ClientSecret = existing.ClientSecret
		}

		s := &domain.SSOSettings{
			Enabled:      body.Enabled,
			IssuerURL:    strings.TrimRight(body.IssuerURL, "/"),
			ClientID:     body.ClientID,
			ClientSecret: body.ClientSecret,
			RedirectURL:  body.RedirectURL,
		}
		if err := c.settingsRepo.Upsert(r.Context(), s, claims.UserID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Invalidate discovery cache since issuer may have changed.
		c.mu.Lock()
		c.oidcMeta = nil
		c.cachedIssuer = ""
		c.mu.Unlock()

		writeOK(w)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleStart redirects the browser to the OIDC authorization endpoint.
func (c *SSOController) handleStart(w http.ResponseWriter, r *http.Request) {
	s := c.activeSettings(r.Context())
	if !s.Enabled || s.ClientID == "" {
		http.Error(w, "SSO not configured", http.StatusServiceUnavailable)
		return
	}

	meta, err := c.discover(s.IssuerURL)
	if err != nil {
		log.Printf("SSO discover (issuer=%q): %v", s.IssuerURL, err)
		http.Error(w, fmt.Sprintf("SSO discover failed (issuer=%q): %v", s.IssuerURL, err), http.StatusServiceUnavailable)
		return
	}

	state, err := randomHex(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: ssoStateCookie, Value: state, Path: "/",
		HttpOnly: true, Secure: isSecureRequest(r),
		MaxAge: 600, SameSite: http.SameSiteLaxMode,
	})

	q := url.Values{}
	q.Set("client_id", s.ClientID)
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("redirect_uri", s.RedirectURL)
	q.Set("state", state)

	http.Redirect(w, r, meta.AuthorizationEndpoint+"?"+q.Encode(), http.StatusFound)
}

// handleCallback processes the OIDC authorization code callback.
func (c *SSOController) handleCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(ssoStateCookie)
	if err != nil || r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: ssoStateCookie, Value: "", Path: "/", MaxAge: -1})

	code := r.URL.Query().Get("code")
	if code == "" {
		errDesc := r.URL.Query().Get("error_description")
		if errDesc == "" {
			errDesc = r.URL.Query().Get("error")
		}
		http.Error(w, "no authorization code: "+errDesc, http.StatusBadRequest)
		return
	}

	s := c.activeSettings(r.Context())
	meta, err := c.discover(s.IssuerURL)
	if err != nil {
		http.Error(w, "SSO not available", http.StatusServiceUnavailable)
		return
	}

	tokens, err := c.exchangeCode(meta.TokenEndpoint, code, s)
	if err != nil {
		log.Printf("SSO token exchange: %v", err)
		http.Error(w, "token exchange failed", http.StatusBadGateway)
		return
	}

	info, err := c.fetchUserinfo(meta.UserinfoEndpoint, tokens.AccessToken)
	if err != nil {
		log.Printf("SSO userinfo: %v", err)
		http.Error(w, "userinfo failed", http.StatusBadGateway)
		return
	}

	if info.Email == "" || info.Sub == "" {
		http.Error(w, "SSO provider did not return email or sub", http.StatusBadGateway)
		return
	}

	ctx := r.Context()

	user, err := c.userRepo.GetBySSOSub(ctx, info.Sub)
	if err != nil {
		user, err = c.userRepo.GetByEmail(ctx, info.Email)
		if err != nil {
			user, err = c.provisionUser(ctx, info)
			if err != nil {
				log.Printf("SSO provision: %v", err)
				http.Error(w, "could not create account", http.StatusInternalServerError)
				return
			}
		}
		if linkErr := c.userRepo.LinkSSO(ctx, user.ID, info.Sub); linkErr != nil {
			log.Printf("SSO link: %v", linkErr)
		}
	}

	if !user.IsActive {
		http.Redirect(w, r, "/?sso_error=inactive", http.StatusFound)
		return
	}

	access, _, _, err := middleware.GenerateTokens(c.cfg.JWTSecret, user.ID, user.Username, user.Role, user.SystemRole)
	if err != nil {
		http.Error(w, "token generation failed", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: middleware.CookieName, Value: access, Path: "/",
		HttpOnly: true, Secure: isSecureRequest(r),
		SameSite: http.SameSiteLaxMode, MaxAge: 24 * 60 * 60,
	})
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

// provisionUser creates a viewer account for first-time SSO users.
func (c *SSOController) provisionUser(ctx context.Context, info *userinfoResponse) (*domain.User, error) {
	displayName := info.Name
	if displayName == "" {
		displayName = strings.Split(info.Email, "@")[0]
	}
	username := strings.Split(info.Email, "@")[0]
	u := &domain.User{
		Username:    username,
		DisplayName: displayName,
		Email:       info.Email,
		SSOSub:      info.Sub,
		Role:        "viewer",
		SystemRole:  "user",
		IsActive:    true,
	}
	if err := c.userRepo.Create(ctx, u); err != nil {
		suffix, _ := randomHex(3)
		u.Username = username + "_" + suffix
		if err2 := c.userRepo.Create(ctx, u); err2 != nil {
			return nil, fmt.Errorf("create user: %w", err2)
		}
	}
	return u, nil
}

// discover fetches (and caches) the OIDC provider metadata for the given issuer.
func (c *SSOController) discover(issuerURL string) (*oidcDiscovery, error) {
	c.mu.RLock()
	if c.oidcMeta != nil && c.cachedIssuer == issuerURL {
		m := c.oidcMeta
		c.mu.RUnlock()
		return m, nil
	}
	c.mu.RUnlock()

	wellKnown := strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"
	resp, err := httpGet(wellKnown)
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	var meta oidcDiscovery
	if err := json.Unmarshal(resp, &meta); err != nil {
		return nil, fmt.Errorf("discovery parse: %w", err)
	}

	c.mu.Lock()
	c.oidcMeta = &meta
	c.cachedIssuer = issuerURL
	c.mu.Unlock()
	return &meta, nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
}

func (c *SSOController) exchangeCode(tokenEndpoint, code string, s *domain.SSOSettings) (*tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", s.RedirectURL)
	form.Set("client_id", s.ClientID)
	form.Set("client_secret", s.ClientSecret)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm(tokenEndpoint, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint %d: %s", resp.StatusCode, body)
	}
	var t tokenResponse
	return &t, json.Unmarshal(body, &t)
}

type userinfoResponse struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

func (c *SSOController) fetchUserinfo(userinfoEndpoint, accessToken string) (*userinfoResponse, error) {
	req, _ := http.NewRequest(http.MethodGet, userinfoEndpoint, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo %d: %s", resp.StatusCode, body)
	}
	var info userinfoResponse
	return &info, json.Unmarshal(body, &info)
}

func httpGet(u string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s → %d", u, resp.StatusCode)
	}
	return body, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
