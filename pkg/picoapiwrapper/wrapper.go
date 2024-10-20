package picoapiwrapper

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

const (
	picoVerifyUsrPath           = "/s2s/v1/user/validate"
	picoRetrieveUsrPurchasePath = "/s2s/v1/user/purchased"
)

var (
	PicoPlatformServer = "https://platform-cn.picovr.com"
	PicoAccessToken    = "PICO|App_id|App_Secret"
)

type PicoApiRepository interface {
	SetupPicoHttpClient(pPlatformServer string, pAccessToken string)
	VerifyPICOUser(pUsr PICOUserVerifyForm) (tokenResp PicoUserVerifyResponse, err error)
	RetrievePicoUserPurchase(pUsr PICOUserPurchaseRetrievalForm) (tokenResp PicoUserPurchaseResponse, err error)
}

func NewPicoApiRepository() PicoApiRepository {
	return &picoApiRepositoryImpl{}
}

type picoApiRepositoryImpl struct {
	picoPlatformServer string
	picoAccessToken    string
}

func (p *picoApiRepositoryImpl) SetupPicoHttpClient(pPlatformServer string, pAccessToken string) {
	log.Printf("SetupPicoHttpClient using %v, %v", pPlatformServer, pAccessToken)
	PicoPlatformServer = pPlatformServer
	PicoAccessToken = pAccessToken
}

type PicoResponseBase struct {
	Code         int    `json:"code"`
	ErrorMessage string `json:"em"`
	TraceID      string `json:"trace_id"`
}

type PicoUserVerifyResponseData struct {
	IsValidate bool `json:"is_validate"`
}

type PicoUserVerifyResponse struct {
	PicoResponseBase
	data PicoUserVerifyResponseData
}

type PICOUserVerifyForm struct {
	UsrID  string `json:"user_id" form:"usr"`
	UsrTkn string `json:"access_token" form:"tkn"`
}

func (p *PICOUserVerifyForm) Bytes() []byte {
	if res, err := json.Marshal(p); err == nil {
		return res
	}

	return []byte{}
}

func (p *picoApiRepositoryImpl) VerifyPICOUser(pUsr PICOUserVerifyForm) (tokenResp PicoUserVerifyResponse, err error) {

	var req *http.Request
	if req, err = http.NewRequest("POST", PicoPlatformServer+picoVerifyUsrPath, bytes.NewBuffer(pUsr.Bytes())); err != nil {
		return
	}

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

	err = json.Unmarshal(respBytes, &tokenResp)

	return
}

type PICOUserPurchaseRetrievalForm struct {
	UsrID  string `json:"user_id" form:"usr"`
	AccTkn string `json:"access_token"`
}

func (p *PICOUserPurchaseRetrievalForm) Bytes() []byte {
	//fill access token if empty
	if len(p.AccTkn) <= 0 {
		p.AccTkn = PicoAccessToken
	}

	if res, err := json.Marshal(p); err == nil {
		return res
	}

	return []byte{}
}

type PicoUserPurchaseResponseData struct {
	SKU               string `json:"sku"`
	PurchaseID        string `json:"purchase_i"`
	GrantTime         int64  `json:"grant_time"`
	ExpirationTime    int64  `json:"expiration_time"`
	AddonsType        int    `json:"addons_type"`
	OuterID           string `json:"outer_id"`
	CurrentPeriodType int    `json:"current_period_type"`
	NextPeriodType    int    `json:"next_period_type"`
	DiscountType      int    `json:"discount_type"`
	NextPayTime       int64  `json:"next_pay_time"`
}

type PicoUserPurchaseResponse struct {
	PicoResponseBase
	data []PicoUserPurchaseResponseData
}

func (p *picoApiRepositoryImpl) RetrievePicoUserPurchase(pUsr PICOUserPurchaseRetrievalForm) (tokenResp PicoUserPurchaseResponse, err error) {

	var req *http.Request
	if req, err = http.NewRequest("POST", PicoPlatformServer+picoRetrieveUsrPurchasePath, bytes.NewBuffer(pUsr.Bytes())); err != nil {
		return
	}

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

	err = json.Unmarshal(respBytes, &tokenResp)

	return
}
