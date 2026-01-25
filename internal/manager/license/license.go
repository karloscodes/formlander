package license

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const GumroadProductID = "wSDPAFPoIbRxe793R6eCCQ=="

// GumroadResponse represents the Gumroad license verification response
type GumroadResponse struct {
	Success  bool   `json:"success"`
	Uses     int    `json:"uses"`
	Purchase struct {
		Email        string `json:"email"`
		ProductID    string `json:"product_id"`
		ProductName  string `json:"product_name"`
		Refunded     bool   `json:"refunded"`
		Chargebacked bool   `json:"chargebacked"`
	} `json:"purchase"`
	Message string `json:"message,omitempty"`
}

// ValidateWithGumroad validates a license key with Gumroad API
func ValidateWithGumroad(licenseKey string) (*GumroadResponse, error) {
	data := url.Values{}
	data.Set("product_id", GumroadProductID)
	data.Set("license_key", licenseKey)

	resp, err := http.PostForm("https://api.gumroad.com/v2/licenses/verify", data)
	if err != nil {
		return nil, fmt.Errorf("failed to contact Gumroad API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var gumroadResp GumroadResponse
	if err := json.Unmarshal(body, &gumroadResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &gumroadResp, nil
}

// Validate checks if a license key is valid
func Validate(licenseKey string) (string, error) {
	resp, err := ValidateWithGumroad(licenseKey)
	if err != nil {
		return "", err
	}

	if !resp.Success {
		return "", fmt.Errorf("invalid license key: %s", resp.Message)
	}

	if resp.Purchase.Refunded {
		return "", fmt.Errorf("this license has been refunded")
	}

	if resp.Purchase.Chargebacked {
		return "", fmt.Errorf("this license has been chargebacked")
	}

	if resp.Purchase.ProductID != GumroadProductID {
		return "", fmt.Errorf("license is for a different product")
	}

	return resp.Purchase.Email, nil
}
