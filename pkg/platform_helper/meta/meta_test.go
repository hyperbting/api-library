package meta

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFormAccessToken_Once(t *testing.T) {
	cfg := &OCULUSPlatformConfig{
		PlatformServer: "https://graph.oculus.com",
		AppID:          "123456",
		AppSecret:      "secret_abc",
	}

	token1 := cfg.FormAccessToken()
	expected := "OC|123456|secret_abc"
	if token1 != expected {
		t.Fatalf("expected token %q, got %q", expected, token1)
	}

	// Mutate credentials to prove sync.Once returns cached token
	cfg.AppID = "changed_id"
	token2 := cfg.FormAccessToken()
	if token2 != expected {
		t.Fatalf("expected cached token %q after mutation, got %q", expected, token2)
	}
}

func TestGenerateSHA256SignatureWithOculusSecret(t *testing.T) {
	cfg := &OCULUSPlatformConfig{
		AppID:     "123456",
		AppSecret: "test_secret",
	}
	repo := NewMetaApiRepository(cfg)

	payload := "test payload"
	sig := repo.GenerateSHA256SignatureWithOculusSecret(payload)

	if !strings.HasPrefix(sig, "sha256=") {
		t.Fatalf("expected signature to start with 'sha256=', got %q", sig)
	}

	// Deterministic HMAC check
	sig2 := repo.GenerateSHA256SignatureWithOculusSecret(payload)
	if sig != sig2 {
		t.Fatalf("expected deterministic signature: %q != %q", sig, sig2)
	}
}

func TestVerifyAttestationToken_ParseClaims(t *testing.T) {
	claims := AttestationClaimsDTO{
		RequestDetails: struct {
			Exp       int64  `json:"exp"`
			Nonce     string `json:"nonce"`
			Timestamp int64  `json:"timestamp"`
		}{
			Exp:       time.Now().Add(1 * time.Hour).Unix(),
			Nonce:     "nonce_123",
			Timestamp: time.Now().Unix(),
		},
		AppState: struct {
			AppIntegrityState       string   `json:"app_integrity_state" enums:"NotEvaluated,StoreRecognized"`
			PackageCertSha256Digest []string `json:"package_cert_sha256_digest"`
			PackageId               string   `json:"package_id"`
			Version                 string   `json:"version"`
		}{
			AppIntegrityState: "StoreRecognized",
			PackageId:         "com.test.app",
			Version:           "1.0.0",
		},
		DeviceState: struct {
			DeviceIntegrityState string `json:"device_integrity_state" enums:"NotTrusted,Advanced"`
			UniqueId             string `json:"unique_id"`
		}{
			DeviceIntegrityState: "Advanced",
			UniqueId:             "device_uid_123",
		},
	}

	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("failed to marshal claims: %v", err)
	}

	b64Claims := base64.RawURLEncoding.EncodeToString(claimsBytes)
	successMsg := "success"

	resp := VerifyAttestationTokenResponseDTO{
		Data: []struct {
			Message        *string `json:"message" enums:"success,invalid signature,token expired"`
			ClaimsInBase64 *string `json:"claims" extensions:"x-format=base64"`
		}{
			{
				Message:        &successMsg,
				ClaimsInBase64: &b64Claims,
			},
		},
	}

	if err := resp.ParseClaims(false); err != nil {
		t.Fatalf("ParseClaims failed: %v", err)
	}

	if resp.Claim == nil {
		t.Fatal("expected Claim to be populated, got nil")
	}

	if resp.Claim.DeviceState.UniqueId != "device_uid_123" {
		t.Errorf("expected UniqueId 'device_uid_123', got %q", resp.Claim.DeviceState.UniqueId)
	}

	if resp.IsDeviceBanned() {
		t.Errorf("expected device not banned when DeviceBan is nil")
	}
}

func TestVerifyAttestationToken_IsDeviceBanned(t *testing.T) {
	// Case 1: Empty Data
	emptyResp := VerifyAttestationTokenResponseDTO{}
	if !emptyResp.IsDeviceBanned() {
		t.Errorf("expected empty response to be treated as banned")
	}

	// Case 2: Message not success
	failMsg := "invalid signature"
	failResp := VerifyAttestationTokenResponseDTO{
		Data: []struct {
			Message        *string `json:"message" enums:"success,invalid signature,token expired"`
			ClaimsInBase64 *string `json:"claims" extensions:"x-format=base64"`
		}{
			{Message: &failMsg},
		},
	}
	if !failResp.IsDeviceBanned() {
		t.Errorf("expected failed message to be treated as banned")
	}

	// Case 3: Claims indicate banned
	bannedClaims := AttestationClaimsDTO{
		DeviceBan: &DeviceBanDTO{
			IsBanned:         true,
			RemainingBanTime: 60,
		},
	}
	bannedBytes, _ := json.Marshal(bannedClaims)
	b64Banned := base64.RawURLEncoding.EncodeToString(bannedBytes)
	successMsg := "success"

	bannedResp := VerifyAttestationTokenResponseDTO{
		Data: []struct {
			Message        *string `json:"message" enums:"success,invalid signature,token expired"`
			ClaimsInBase64 *string `json:"claims" extensions:"x-format=base64"`
		}{
			{
				Message:        &successMsg,
				ClaimsInBase64: &b64Banned,
			},
		},
	}

	if !bannedResp.IsDeviceBanned() {
		t.Errorf("expected device to be banned")
	}
}

func TestFormUrl_QueryParams(t *testing.T) {
	cfg := &OCULUSPlatformConfig{
		PlatformServer: "https://graph.oculus.com",
		AppID:          "123",
		AppSecret:      "abc",
	}
	accessToken := cfg.FormAccessToken()

	t.Run("VerifyAttestationTokenQuery", func(t *testing.T) {
		q := NewVerifyAttestationTokenQueryDTO("token_xyz")
		urlStr := q.formUrl(cfg)

		parsed, err := url.Parse(urlStr)
		if err != nil {
			t.Fatalf("invalid url: %v", err)
		}
		if parsed.Path != VerifyAttestationTokenPath {
			t.Errorf("expected path %q, got %q", VerifyAttestationTokenPath, parsed.Path)
		}
		if parsed.Query().Get("token") != "token_xyz" {
			t.Errorf("expected token query 'token_xyz', got %q", parsed.Query().Get("token"))
		}
		if parsed.Query().Get("access_token") != accessToken {
			t.Errorf("expected access_token query %q, got %q", accessToken, parsed.Query().Get("access_token"))
		}
	})

	t.Run("BanStatusRequestDTO_WithBanId", func(t *testing.T) {
		q := BanStatusRequestDTO{BanId: "ban_001"}
		urlStr := q.formUrl(cfg)

		parsed, err := url.Parse(urlStr)
		if err != nil {
			t.Fatalf("invalid url: %v", err)
		}
		if parsed.Path != DeviceBanStatusCheckPath {
			t.Errorf("expected path %q, got %q", DeviceBanStatusCheckPath, parsed.Path)
		}
		if parsed.Query().Get("ban_id") != "ban_001" {
			t.Errorf("expected ban_id 'ban_001', got %q", parsed.Query().Get("ban_id"))
		}
		if parsed.Query().Has("unique_id") {
			t.Errorf("did not expect unique_id when BanId is set")
		}
	})

	t.Run("BanStatusRequestDTO_WithUniqueId", func(t *testing.T) {
		q := BanStatusRequestDTO{UniqueId: "uid_999"}
		urlStr := q.formUrl(cfg)

		parsed, err := url.Parse(urlStr)
		if err != nil {
			t.Fatalf("invalid url: %v", err)
		}
		if parsed.Query().Get("unique_id") != "uid_999" {
			t.Errorf("expected unique_id 'uid_999', got %q", parsed.Query().Get("unique_id"))
		}
	})

	t.Run("DeviceBanRequestDTO_BanActive", func(t *testing.T) {
		q := DeviceBanRequestDTO{
			DeviceBanDTO: DeviceBanDTO{
				IsBanned:         true,
				RemainingBanTime: 120,
			},
			UniqueId: "uid_123",
		}
		urlStr := q.formUrl(cfg)

		parsed, err := url.Parse(urlStr)
		if err != nil {
			t.Fatalf("invalid url: %v", err)
		}
		if parsed.Path != BanWithAttestationPath {
			t.Errorf("expected path %q, got %q", BanWithAttestationPath, parsed.Path)
		}
		if parsed.Query().Get("is_banned") != "true" {
			t.Errorf("expected is_banned 'true', got %q", parsed.Query().Get("is_banned"))
		}
		if parsed.Query().Get("remaining_time_in_minute") != "120" {
			t.Errorf("expected remaining_time_in_minute '120', got %q", parsed.Query().Get("remaining_time_in_minute"))
		}
	})
}

func TestMetaAttestationClient_MockServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case VerifyAttestationTokenPath:
			successMsg := "success"
			claimsJson := `{"device_state":{"unique_id":"device_mock"}}`
			b64 := base64.RawURLEncoding.EncodeToString([]byte(claimsJson))
			resp := VerifyAttestationTokenResponseDTO{
				Data: []struct {
					Message        *string `json:"message" enums:"success,invalid signature,token expired"`
					ClaimsInBase64 *string `json:"claims" extensions:"x-format=base64"`
				}{
					{
						Message:        &successMsg,
						ClaimsInBase64: &b64,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case DeviceBanStatusCheckPath:
			resp := BanStatusResponseDTO{
				Data: []BanStatusDataDTO{
					{
						Message:                "ok",
						IsBanned:               false,
						RemainingTimeInMinutes: 0,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case BanWithAttestationPath:
			resp := BanResponseDTO{
				Message: "Success",
				BanID:   "ban_mock_1",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	cfg := &OCULUSPlatformConfig{
		PlatformServer: ts.URL,
		AppID:          "mock_app",
		AppSecret:      "mock_secret",
	}
	client := NewMetaAttestationClient(cfg)
	ctx := context.Background()

	t.Run("VerifyAttestationToken", func(t *testing.T) {
		res, err := client.RequestOculusVerifyAttestationToken(ctx, NewVerifyAttestationTokenQueryDTO("mock_token"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Data) == 0 || *res.Data[0].Message != "success" {
			t.Errorf("unexpected verify response: %+v", res)
		}
	})

	t.Run("AttestationBanStatus", func(t *testing.T) {
		res, err := client.RequestOculusAttestationBanStatus(ctx, &BanStatusRequestDTO{UniqueId: "mock_uid"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Data) == 0 || res.Data[0].IsBanned {
			t.Errorf("expected not banned, got: %+v", res)
		}
	})

	t.Run("AttestationBan", func(t *testing.T) {
		res, err := client.RequestOculusAttestationBan(ctx, &DeviceBanRequestDTO{
			DeviceBanDTO: DeviceBanDTO{IsBanned: true, RemainingBanTime: 30},
			UniqueId:  "mock_uid",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.BanID != "ban_mock_1" {
			t.Errorf("expected BanID 'ban_mock_1', got %q", res.BanID)
		}
	})
}
