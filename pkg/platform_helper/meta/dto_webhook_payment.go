package meta
import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
)

type NotificationType string
type FieldType string

const (
	Purchased NotificationType = "PURCHASED"
	Refunded  NotificationType = "REFUNDED"

	FieldJoinIntent  FieldType = "join_intent"
	FieldOrderStatus FieldType = "order_status"
)

// ProductInfoDTO struct with enforced NotificationType
type ProductInfoDTO struct {
	NotificationType NotificationType `json:"notification_type"`
	ReportingID      string                 `json:"reporting_id"`
	SKU              string                 `json:"sku"`
	DeveloperPayload string                 `json:"developer_payload"`
}

// Validate the notification type
func (p *ProductInfoDTO) Validate() (errMsgs []string) {

	switch p.NotificationType {
		case Purchased, Refunded:
		return
		default:
		errMsgs = append(errMsgs, "invalid notification type")
	}
	return
}

// ParseDeveloperPayload 將序列化的 DeveloperPayload 字串解析為 DTO
func (p *ProductInfoDTO) ParseDeveloperPayload() (*DeveloperPayloadDTO, error) {
	if p.DeveloperPayload == "" {
		return nil, ErrEmptyDeveloperLoad
	}

	var payload DeveloperPayloadDTO
	if err := json.Unmarshal([]byte(p.DeveloperPayload), &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal developer_payload: %w", err)
	}

	return &payload, nil
}

type ValueDTO struct {
	EventTime   int64            `json:"event_time"`
	UserID      string            `json:"user_id"`
	ProductInfo ProductInfoDTO `json:"product_info"`
}

type OrderStatusDTO struct {
	Field FieldType `json:"field"`
	Value ValueDTO     `json:"value"`
}

type DeveloperPayloadDTO struct {
	Secret string `json:"secret"`
}

// VerifySecret 使用嚴格常數時間比較，防止 Timing Attack
func (d *DeveloperPayloadDTO) VerifySecret(expectedSecret string) bool {
	if len(d.Secret) == 0 || len(expectedSecret) == 0 {
		return false
	}
	// subtle.ConstantTimeCompare 會自動比對長度與內容，維持相同的比對時間
	return subtle.ConstantTimeCompare([]byte(d.Secret), []byte(expectedSecret)) == 1
}
