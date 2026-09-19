package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MetaAttestationClient handles Meta App Attestation & Device Ban platform APIs.
type MetaAttestationClient interface {
	RequestOculusVerifyAttestationToken(ctx context.Context, q VerifyAttestationTokenQueryDTO) (VerifyAttestationTokenResponseDTO, error)
	RequestOculusAttestationBanStatus(ctx context.Context, q *BanStatusRequestDTO) (BanStatusResponseDTO, error)
	RequestOculusAttestationBan(ctx context.Context, q *DeviceBanRequestDTO) (BanResponseDTO, error)
}

type metaAttestationClientImpl struct {
	cfg        *OCULUSPlatformConfig
	httpClient *http.Client
}

func NewMetaAttestationClient(cfg *OCULUSPlatformConfig) MetaAttestationClient {
	return &metaAttestationClientImpl{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *metaAttestationClientImpl) RequestOculusVerifyAttestationToken(ctx context.Context, q VerifyAttestationTokenQueryDTO) (oculusResp VerifyAttestationTokenResponseDTO, err error) {
	// https://developers.meta.com/horizon/documentation/unity/ps-attestation-api

	var req *http.Request

	if req, err = http.NewRequestWithContext(ctx, http.MethodGet, q.formUrl(c.cfg), nil); err != nil {
		return
	}

	// Send request
	var resp *http.Response
	if resp, err = c.httpClient.Do(req); err != nil {
		return
	}

	defer resp.Body.Close()

	var respBytes []byte
	if respBytes, err = io.ReadAll(resp.Body); err != nil {
		return
	}
	err = json.Unmarshal(respBytes, &oculusResp)

	return
}

func (c *metaAttestationClientImpl) RequestOculusAttestationBanStatus(ctx context.Context, q *BanStatusRequestDTO) (oculusResp BanStatusResponseDTO, err error) {
	// https://developers.meta.com/horizon/documentation/spatial-sdk/ps-attestation-api/#how-to-ban-a-device

	var req *http.Request

	if req, err = http.NewRequestWithContext(ctx, http.MethodGet, q.formUrl(c.cfg), nil); err != nil {
		return
	}

	// Send request
	var resp *http.Response
	if resp, err = c.httpClient.Do(req); err != nil {
		return
	}

	defer resp.Body.Close()

	var respBytes []byte
	if respBytes, err = io.ReadAll(resp.Body); err != nil {
		return
	}
	err = json.Unmarshal(respBytes, &oculusResp)

	return
}

func (c *metaAttestationClientImpl) RequestOculusAttestationBan(ctx context.Context, q *DeviceBanRequestDTO) (oculusResp BanResponseDTO, err error) {
	// https://developers.meta.com/horizon/documentation/spatial-sdk/ps-attestation-api/#how-to-ban-a-device

	reqURL := q.formUrl(c.cfg)

	// Pass context for proper timeout/cancellation handling
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return oculusResp, fmt.Errorf("failed to create ban request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return oculusResp, fmt.Errorf("oculus ban api request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return oculusResp, fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle non-200 responses explicitly
	if resp.StatusCode != http.StatusOK {
		return oculusResp, fmt.Errorf("oculus attestation ban error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	if err := json.Unmarshal(respBytes, &oculusResp); err != nil {
		return oculusResp, fmt.Errorf("failed to unmarshal oculus ban response: %w", err)
	}

	return oculusResp, nil
}
