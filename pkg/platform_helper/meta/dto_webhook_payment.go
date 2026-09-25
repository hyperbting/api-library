package meta

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type NotificationType string
type FieldType string

const (
	Purchased NotificationType = "PURCHASED"
	Refunded  NotificationType = "REFUNDED"

	FieldJoinIntent  FieldType = "join_intent"
	FieldOrderStatus FieldType = "order_status"
)

/*
{
  "field": "order_status",
  "value": {
    "event_time": "1659742639",
    "user_id": "10149999707612630",
    "product_info": {
      "notification_type": "PURCHASED",
      "reporting_id": "03f8833e-9c02-4fa0-978f-4cfe91f86bae",
      "sku": "item_sku_1",
      "developer_payload": "{\"quoteId\": 1234567}"
    }
  }
}
*/

// ProductInfo struct with enforced NotificationType
type ProductInfo struct {
	NotificationType NotificationType `json:"notification_type"`
	ReportingID      string           `json:"reporting_id"`
	SKU              string           `json:"sku"`
	DeveloperPayload string           `json:"developer_payload"`
}

// Validate the notification type
func (p *ProductInfo) Validate() (errMsgs []string) {

	switch p.NotificationType {
	case Purchased, Refunded:
		return
	default:
		errMsgs = append(errMsgs, "invalid notification type")
	}
	return
}

func (p *ProductInfo) ParseDeveloperPayload() (*DeveloperPayload, error) {
	if strings.TrimSpace(p.DeveloperPayload) == "" {
		return nil, fmt.Errorf("developer_payload is empty")
	}

	var devPayload DeveloperPayload
	if err := json.Unmarshal([]byte(p.DeveloperPayload), &devPayload); err != nil {
		return nil, fmt.Errorf("unmarshal failed: %w", err)
	}

	if err := devPayload.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &devPayload, nil
}

type DeveloperPayload struct {
	QuoteID int64 `json:"quoteId"`
}

func (d *DeveloperPayload) Validate() error {
	if d.QuoteID <= 0 {
		return fmt.Errorf("quoteId must be greater than 0")
	}
	return nil
}

// OrderValue contains the event details and product info
type OrderValue struct {
	EventTime   string      `json:"event_time"`
	UserID      string      `json:"user_id"`
	ProductInfo ProductInfo `json:"product_info"`
}

type WebhookEvent struct {
	Field FieldType  `json:"field"`
	Value OrderValue `json:"value"`
}

func (w *WebhookEvent) IsOrderStatusEvent() bool {
	return w.Field == FieldOrderStatus
}
func (e *WebhookEvent) GetOrderStatusValue() (*OrderValue, error) {
	if !e.IsOrderStatusEvent() {
		return nil, errors.New("event is not an order_status field")
	}
	return &e.Value, nil
}
