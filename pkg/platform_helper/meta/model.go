package meta

import (
	"log"
	"net/url"
	"strings"
	"time"
)

// --- Shared Base Responses ---

type OCULUSResponseError struct {
	Message      string `json:"message,omitempty"`
	Type         string `json:"type,omitempty"`
	Code         int    `json:"code,omitempty"`
	ErrorSubcode int    `json:"error_subcode,omitempty"`
	FBTraceID    string `json:"fbtrace_id,omitempty"`
}

type OCULUSResponseBase struct {
	Success bool                `json:"success"`
	Error   OCULUSResponseError `json:"error,omitempty"`
}

type OculusError struct {
	Message      string            `json:"message"`
	Type         string            `json:"type"`
	Code         int               `json:"code"`
	ErrorData    map[string]string `json:"error_data"`
	ErrorSubcode int               `json:"error_subcode"`
	FbTraceID    string            `json:"fbtrace_id"`
}

// --- Entitlements & Items ---

type VerifyItemOwnershipQuery struct {
	SKU   string `json:"sku"`
	UsrID string `json:"user_id"`
}

func (v *VerifyItemOwnershipQuery) BuildQuery(cfg OCULUSPlatformConfig) url.Values {
	params := url.Values{}
	params.Add("sku", v.SKU)
	params.Add("access_token", cfg.FormAccessToken())
	log.Println(params)
	return params
}

type OculusConsumeIAPItemQuery struct {
	VerifyItemOwnershipQuery
}

type RetrieveItemsOwnedQuery struct {
	OrgScopedID string   `json:"user_id"`
	Fields      []string `json:"fields"`
}

func (r *RetrieveItemsOwnedQuery) BuildQuery(cfg OCULUSPlatformConfig) url.Values {
	params := url.Values{}
	params.Add("access_token", cfg.FormAccessToken())
	params.Add("user_id", r.OrgScopedID)
	params.Add("fields", strings.Join(r.Fields, ","))
	return params
}

type OculusItem struct {
	SKU string `json:"sku"`
	ID  string `json:"id"`
}

type OculusData struct {
	ID             string     `json:"id"`
	GrantTime      int64      `json:"grant_time"`
	ExpirationTime int64      `json:"expiration_time"`
	Item           OculusItem `json:"item"`
}

type OculusCursors struct {
	After  string `json:"after"`
	Before string `json:"before"`
}

type OculusPaging struct {
	Cursors  OculusCursors `json:"cursors"`
	Previous string        `json:"previous"`
	Next     string        `json:"next"`
}

type RetrieveItemsOwnedResponse struct {
	Data   []OculusData `json:"data"`
	Paging OculusPaging `json:"paging"`
	Error  OculusError  `json:"error,omitempty"`
}

// --- Nonce Validation ---

type UserNonceValidateQuery struct {
	UserID         string        `json:"user_id"`
	Nonce          string        `json:"nonce"`
	AccessToken    string        `json:"access_token"`
	RequestTimeout time.Duration `json:"timeout"`
}

func (u *UserNonceValidateQuery) BuildWithoutTimeout(accTkn, uid, nonce string) {
	u.AccessToken = accTkn
	u.UserID = uid
	u.Nonce = nonce
}

func (u *UserNonceValidateQuery) Build(accTkn, uid, nonce string) {
	u.BuildWithoutTimeout(accTkn, uid, nonce)
	u.RequestTimeout = requestTimeout
}

func (u *UserNonceValidateQuery) BuildParameter() string {
	parameters := url.Values{}
	parameters.Add("access_token", u.AccessToken)
	parameters.Add("user_id", u.UserID)
	parameters.Add("nonce", u.Nonce)
	return parameters.Encode()
}

type UserNonceValidateResponse struct {
	IsValid bool                `json:"is_valid"`
	Error   OCULUSResponseError `json:"error"`
}

// --- Org Scoped ID ---

type GetOculusOrgScopedIDResponseQuery struct {
	Fields []string `json:"fields"`
}

func (r *GetOculusOrgScopedIDResponseQuery) BuildQuery(cfg OCULUSPlatformConfig) url.Values {
	params := url.Values{}
	params.Add("access_token", cfg.FormAccessToken())
	params.Add("fields", strings.Join(r.Fields, ","))
	log.Println(params)
	return params
}

type GetOculusOrgScopedIDResponse struct {
	ID       string `json:"id"`
	Alias    string `json:"alias"`
	ScopedID string `json:"org_scoped_id"`
}

func (r *GetOculusOrgScopedIDResponse) IsValid() bool {
	return len(r.ScopedID) > 0 && len(r.ID) > 0
}
