package metaapiwrapper

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	UserNonceValidateUrl   = "/user_nonce_validate"
	VerifyItemOwnershipUrl = "/verify_entitlement"
	ConsumeIAPItemUrl      = "/consume_entitlement"
	RetrieveItemsOwnedUrl  = "/viewer_purchases"

	OculusPlatformServer = "https://graph.oculus.com"
)

var (
	requestTimeout = 5 * time.Second
)

type MetaApiRepository interface {
	GenerateSHA256SignatureWithOculusSecret(devPayload string) string
	GetOculusOrgScopedID(oculusUsrID string, q GetOculusOrgScopedIDResponseQuery) (respOrgScopedID GetOculusOrgScopedIDResponse, err error)
	RequestOculusUserNonceValidate(q UserNonceValidateQuery) (OculusResp UserNonceValidateResponse, err error)
	RequestOculusRetrieveItemsOwned(q RetrieveItemsOwnedQuery) (oculusResp RetrieveItemsOwnedResponse, err error)
	RequestOculusVerifyItemOwnership(q VerifyItemOwnershipQuery) (OculusResp OCULUSResponseBase, err error)
}

func NewMetaApiRepository() MetaApiRepository {
	return &metaApiRepositoryImpl{}
}

type metaApiRepositoryImpl struct {
	AccessToken OCULUSPlatformConfig
}

type OCULUSPlatformConfig struct {
	AppID     string
	AppSecret string
}

func (c *OCULUSPlatformConfig) FormAccessToken() (oculusPlatformAccessToken string) {
	oculusPlatformAccessToken = fmt.Sprintf("OC|%v|%v", c.AppID, c.AppSecret)
	log.Printf("FormAccessToken using %v %v: %v", c.AppID, c.AppSecret, oculusPlatformAccessToken)
	return
}

type OCULUSResponseError struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	Code    int    `json:"code,omitempty"`
	//ErrorDate    []string `json:"error_data,omitempty"`
	ErrorSubcode int    `json:"error_subcode,omitempty"`
	FBTraceID    string `json:"fbtrace_id,omitempty"`
}

type OCULUSResponseBase struct {
	Success bool `json:"success"`
	Error   OCULUSResponseError
}

//func SetupOculusHttpClient(oPlatformServer string) {
//	OculusPlatformServer = fmt.Sprintf("%v", oPlatformServer)
//	log.Printf("SetupOculusHttpClient using %v: %v", oPlatformServer, OculusPlatformServer)
//}

type VerifyItemOwnershipQuery struct {
	//UserID      string `json:"user_id"`
	SKU   string `json:"sku"`
	UsrID string `json:"user_id"`
	//AccessToken string `json:"access_token"`
}

func (v *VerifyItemOwnershipQuery) BuildQuery(cfg OCULUSPlatformConfig) url.Values {
	params := url.Values{}
	params.Add("sku", v.SKU)
	params.Add("access_token", cfg.FormAccessToken())

	log.Println(params)
	return params
}

func (m *metaApiRepositoryImpl) RequestOculusVerifyItemOwnership(q VerifyItemOwnershipQuery) (OculusResp OCULUSResponseBase, err error) {
	var req *http.Request

	// OculusPlatformServer+VerifyItemOwnershipUrl+"?"+q.BuildQuery().Encode()
	if req, err = http.NewRequest("GET", fmt.Sprintf("%v/%v%v?%v", OculusPlatformServer, m.AccessToken.AppID, VerifyItemOwnershipUrl, q.BuildQuery(m.AccessToken).Encode()), nil); err != nil {
		return
	}
	log.Printf("RequestOculusVerifyItemOwnership : %v", req.URL)

	// Send request
	client := http.Client{}
	var resp *http.Response
	if resp, err = client.Do(req); err != nil {
		return
	}

	defer resp.Body.Close()

	var respBytes []byte
	if respBytes, err = io.ReadAll(resp.Body); err != nil {
		return
	}

	err = json.Unmarshal(respBytes, &OculusResp)

	return
}

type RetrieveItemsOwnedQuery struct {
	OrgScopedID string   `json:"user_id"`
	Fields      []string `json:"fields"`
}

func (r *RetrieveItemsOwnedQuery) BuildQuery(cfg OCULUSPlatformConfig) url.Values {
	params := url.Values{}
	params.Add("access_token", cfg.FormAccessToken()) //params.Add("access_token", r.AccessToken)
	params.Add("user_id", r.OrgScopedID)
	params.Add("fields", strings.Join(r.Fields, ","))

	//log.Println(params)
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

type OculusError struct {
	Message      string            `json:"message"`
	Type         string            `json:"type"`
	Code         int               `json:"code"`
	ErrorData    map[string]string `json:"error_data"`
	ErrorSubcode int               `json:"error_subcode"`
	FbTraceID    string            `json:"fbtrace_id"`
}

type RetrieveItemsOwnedResponse struct {
	Data   []OculusData `json:"data"`
	Paging OculusPaging `json:"paging"`
	Error  OculusError  `json:"error,omitempty"`
}

func (m *metaApiRepositoryImpl) RequestOculusRetrieveItemsOwned(q RetrieveItemsOwnedQuery) (oculusResp RetrieveItemsOwnedResponse, err error) {
	var req *http.Request

	//https://developer.oculus.com/documentation/unity/ps-iap-s2s/
	// OculusPlatformServer+RetrieveItemsOwnedUrl+"?"+q.BuildQuery().Encode()
	if req, err = http.NewRequest("GET", fmt.Sprintf("%v/%v%v?%v", OculusPlatformServer, m.AccessToken.AppID, RetrieveItemsOwnedUrl, q.BuildQuery(m.AccessToken).Encode()), nil); err != nil {
		return
	}
	//log.Printf("RequestOculusRetrieveItemsOwned : %v", req.URL)

	// Send request
	client := http.Client{}
	var resp *http.Response
	if resp, err = client.Do(req); err != nil {
		return
	}

	defer resp.Body.Close()

	var respBytes []byte
	if respBytes, err = io.ReadAll(resp.Body); err != nil {
		return
	}

	if err = json.Unmarshal(respBytes, &oculusResp); err != nil {
		return
	}

	if len(oculusResp.Error.Message) > 0 {
		err = errors.New(oculusResp.Error.Message)
	}

	return
}

//type ConsumeIAPItemQuery struct {
//	SKU         string `json:"sku"`
//	AccessToken string `json:"access_token"`
//}
//
//func (c *ConsumeIAPItemQuery) BuildQuery() url.Values {
//	params := url.Values{}
//	params.Add("sku", c.SKU)
//	params.Add("access_token", c.AccessToken)
//
//	log.Println(params)
//	return params
//}

type OculusConsumeIAPItemQuery struct {
	VerifyItemOwnershipQuery
}

func (m *metaApiRepositoryImpl) RequestOculusConsumeIAPItem(q OculusConsumeIAPItemQuery) (OculusResp OCULUSResponseBase, err error) {
	var req *http.Request

	if req, err = http.NewRequest("GET", fmt.Sprintf("%v/%v%v?%v", OculusPlatformServer, m.AccessToken.AppID, ConsumeIAPItemUrl, q.BuildQuery(m.AccessToken).Encode()), nil); err != nil {
		return
	}
	log.Printf("RequestOculusConsumeIAPItem : %v", req.URL)

	// Send request
	client := http.Client{}
	var resp *http.Response
	if resp, err = client.Do(req); err != nil {
		return
	}

	defer resp.Body.Close()

	var respBytes []byte
	if respBytes, err = io.ReadAll(resp.Body); err != nil {
		return
	}

	err = json.Unmarshal(respBytes, &OculusResp)

	return
}

type UserNonceValidateQuery struct {
	UserID      string `json:"user_id"`
	Nonce       string `json:"nonce"`
	AccessToken string `json:"access_token"`

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

	// Add parameters to the URL
	parameters.Add("access_token", u.AccessToken)
	parameters.Add("user_id", u.UserID)
	parameters.Add("nonce", u.Nonce)

	return parameters.Encode()
}

type UserNonceValidateResponse struct {
	IsValid bool                `json:"is_valid"`
	Error   OCULUSResponseError `json:"error"`
}

func (m *metaApiRepositoryImpl) RequestOculusUserNonceValidate(q UserNonceValidateQuery) (OculusResp UserNonceValidateResponse, err error) {
	var req *http.Request

	// Combine the base URL, path, and parameters
	fullURL := fmt.Sprintf("%s%s?%s", OculusPlatformServer, UserNonceValidateUrl, q.BuildParameter())

	if req, err = http.NewRequest("POST", fullURL, nil); err != nil {
		return
	}
	//log.Printf("RequestOculusUserNonceValidate : %v", req.URL)

	// Send request
	client := http.Client{}
	if q.RequestTimeout != 0 {
		client.Timeout = q.RequestTimeout // Timeout set to 5 seconds by default
	}

	var resp *http.Response
	if resp, err = client.Do(req); err != nil {
		return
	}

	defer resp.Body.Close()

	var respBytes []byte
	if respBytes, err = io.ReadAll(resp.Body); err != nil {
		return
	}

	//log.Printf("%#v", string(respBytes))
	if err = json.Unmarshal(respBytes, &OculusResp); err != nil {
		//log.Println(string(respBytes))
		return
	}

	if !OculusResp.IsValid {
		err = gorm.ErrRecordNotFound
	}

	return
}

type GetOculusOrgScopedIDResponseQuery struct {
	Fields []string `json:"fields"`
	//AccessToken string   `json:"access_token"`
}

func (r *GetOculusOrgScopedIDResponseQuery) BuildQuery(cfg OCULUSPlatformConfig) url.Values {
	params := url.Values{}
	params.Add("access_token", cfg.FormAccessToken()) //r.AccessToken)
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
	if len(r.ScopedID) <= 0 || len(r.ID) <= 0 {
		return false
	}
	return true
}

// GetOculusOrgScopedID from Oculus usrID to Oculus Verified Org Scoped ID
// the ID is actually desired value; input oculusUsrID will be in GetOculusOrgScopedIDResponse.ScopedID
func (m *metaApiRepositoryImpl) GetOculusOrgScopedID(oculusUsrID string, q GetOculusOrgScopedIDResponseQuery) (respOrgScopedID GetOculusOrgScopedIDResponse, err error) {
	//https://developer.oculus.com/documentation/unity/ps-ownership#retrieve-a-verified-org-scoped-id

	var req *http.Request

	if req, err = http.NewRequest("GET", fmt.Sprintf("%v/%v?%v", OculusPlatformServer, oculusUsrID, q.BuildQuery(m.AccessToken).Encode()), nil); err != nil {
		return
	}
	//log.Printf("GetOculusOrgScopedID : %v", req.URL)

	// Send request
	client := http.Client{}
	var resp *http.Response
	if resp, err = client.Do(req); err != nil {
		return
	}

	defer resp.Body.Close()

	var respBytes []byte
	if respBytes, err = io.ReadAll(resp.Body); err != nil {
		return
	}

	//log.Printf("GetOculusOrgScopedID : %v", string(respBytes))

	if err = json.Unmarshal(respBytes, &respOrgScopedID); err != nil {
		return
	}

	if !respOrgScopedID.IsValid() {
		err = gorm.ErrRecordNotFound
	}
	return
}

func (m *metaApiRepositoryImpl) GenerateSHA256SignatureWithOculusSecret(devPayload string) string {
	// Create a new HMAC using SHA256
	h := hmac.New(sha256.New, []byte(m.AccessToken.AppSecret))

	// Write the payload to the HMAC
	h.Write([]byte(devPayload))

	// Compute the HMAC signature
	signature := h.Sum(nil)

	// Return the signature as a hexadecimal string
	return "sha256=" + hex.EncodeToString(signature)
}
