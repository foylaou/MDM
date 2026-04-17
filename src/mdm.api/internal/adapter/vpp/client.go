package vpp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	sToken     string
	httpClient *http.Client
}

func NewClient(tokenPath string) (*Client, error) {
	if tokenPath == "" {
		return &Client{httpClient: &http.Client{Timeout: 30 * time.Second}}, nil
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("vpp: read token: %w", err)
	}
	return &Client{
		sToken:     strings.TrimSpace(string(data)),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) AssignLicense(ctx context.Context, adamID string, serialNumbers []string) (string, error) {
	return c.manageLicenses(ctx, adamID, serialNumbers, true)
}

func (c *Client) RevokeLicense(ctx context.Context, adamID string, serialNumbers []string) (string, error) {
	return c.manageLicenses(ctx, adamID, serialNumbers, false)
}

func (c *Client) manageLicenses(ctx context.Context, adamID string, serialNumbers []string, assign bool) (string, error) {
	if c.sToken == "" {
		return "", fmt.Errorf("vpp: no token configured")
	}
	payload := map[string]interface{}{
		"sToken":     c.sToken,
		"adamIdStr":  adamID,
	}
	if assign {
		payload["associateSerialNumbers"] = serialNumbers
	} else {
		payload["disassociateSerialNumbers"] = serialNumbers
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://vpp.itunes.apple.com/mdm/manageVPPLicensesByAdamIdSrv",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vpp: HTTP %d: %s", resp.StatusCode, string(data))
	}

	// Validate Apple VPP response
	var result struct {
		Status       int    `json:"status"`
		ErrorNumber  int    `json:"errorNumber"`
		ErrorMessage string `json:"errorMessage"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("vpp: invalid response: %s", string(data))
	}
	if result.Status != 0 {
		return "", fmt.Errorf("vpp: error %d: %s", result.ErrorNumber, result.ErrorMessage)
	}

	return string(data), nil
}
