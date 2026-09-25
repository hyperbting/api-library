package meta

import (
	"fmt"
	"strconv"
	"strings"

	"api-library/pkg/platform_helper/meta"
)

// ProcessOrderDTO 屬於內部業務層所需的轉譯後資料
type ProcessOrderDTO struct {
	UserID           string                `json:"userId"`
	SKU              string                `json:"sku"`
	QuoteID          int64                 `json:"quoteId"`
	ReportingID      string                `json:"reportingId"`
	EventTime        int64                 `json:"eventTime"`
	NotificationType meta.NotificationType `json:"notificationType"`
}

type WebhookPaymentService interface {
	ParseAndTransform(event *meta.WebhookEvent) (*ProcessOrderDTO, error)
}

type metaWebhookPaymentServiceImpl struct{}

func NewWebhookPaymentService() WebhookPaymentService {
	return &metaWebhookPaymentServiceImpl{}
}

// ParseAndTransform 將 Meta 原生 Webhook 轉譯為內部 ProcessOrderDTO
func (s *metaWebhookPaymentServiceImpl) ParseAndTransform(event *meta.WebhookEvent) (*ProcessOrderDTO, error) {
	if event == nil {
		return nil, fmt.Errorf("webhook event is nil")
	}

	if !event.IsOrderStatusEvent() {
		return nil, fmt.Errorf("unsupported field: %s", event.Field)
	}

	val := event.Value
	pInfo := val.ProductInfo

	// 1. 驗證 NotificationType
	if errs := pInfo.Validate(); len(errs) > 0 {
		return nil, fmt.Errorf("invalid product info: %s", strings.Join(errs, ", "))
	}

	// 2. 解析內層 DeveloperPayload (包含 Validate 邏輯)
	devPayload, err := pInfo.ParseDeveloperPayload()
	if err != nil {
		return nil, fmt.Errorf("failed to parse developer_payload: %w", err)
	}

	// 3. 轉譯 Unix Timestamp (string -> int64)
	eventTime, err := strconv.ParseInt(val.EventTime, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid event_time format: %w", err)
	}

	// 4. 組裝成乾淨的內部 DTO
	return &ProcessOrderDTO{
		UserID:           val.UserID,
		SKU:              pInfo.SKU,
		QuoteID:          devPayload.QuoteID,
		ReportingID:      pInfo.ReportingID,
		EventTime:        eventTime,
		NotificationType: pInfo.NotificationType,
	}, nil
}
