package meta

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"gorm.io/gorm"
)

type MetaApiRepository interface {
	GenerateSHA256SignatureWithOculusSecret(devPayload string) string
	GetOculusOrgScopedID(oculusUsrID string, q GetOculusOrgScopedIDResponseQuery) (respOrgScopedID GetOculusOrgScopedIDResponse, err error)
	RequestOculusUserNonceValidate(q UserNonceValidateQuery) (OculusResp UserNonceValidateResponse, err error)
	RequestOculusRetrieveItemsOwned(q RetrieveItemsOwnedQuery) (oculusResp RetrieveItemsOwnedResponse, err error)
	RequestOculusVerifyItemOwnership(q VerifyItemOwnershipQuery) (OculusResp OCULUSResponseBase, err error)
	RequestOculusConsumeIAPItem(q OculusConsumeIAPItemQuery) (OculusResp OCULUSResponseBase, err error)
}

func NewMetaApiRepository(cfg OCULUSPlatformConfig) MetaApiRepository {
	return &metaApiRepositoryImpl{
		AccessToken: cfg,
	}
}

type metaApiRepositoryImpl struct {
	AccessToken OCULUSPlatformConfig
}

func (m *metaApiRepositoryImpl) RequestOculusVerifyItemOwnership(q VerifyItemOwnershipQuery) (OculusResp OCULUSResponseBase, err error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%v/%v%v?%v", OculusPlatformServer, m.AccessToken.AppID, VerifyItemOwnershipUrl, q.BuildQuery(m.AccessToken).Encode()), nil)
	if err != nil {
		return
	}
	log.Printf("RequestOculusVerifyItemOwnership : %v", req.URL)

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	err = json.Unmarshal(respBytes, &OculusResp)
	return
}

func (m *metaApiRepositoryImpl) RequestOculusRetrieveItemsOwned(q RetrieveItemsOwnedQuery) (oculusResp RetrieveItemsOwnedResponse, err error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%v/%v%v?%v", OculusPlatformServer, m.AccessToken.AppID, RetrieveItemsOwnedUrl, q.BuildQuery(m.AccessToken).Encode()), nil)
	if err != nil {
		return
	}

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
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

func (m *metaApiRepositoryImpl) RequestOculusConsumeIAPItem(q OculusConsumeIAPItemQuery) (OculusResp OCULUSResponseBase, err error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%v/%v%v?%v", OculusPlatformServer, m.AccessToken.AppID, ConsumeIAPItemUrl, q.BuildQuery(m.AccessToken).Encode()), nil)
	if err != nil {
		return
	}
	log.Printf("RequestOculusConsumeIAPItem : %v", req.URL)

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	err = json.Unmarshal(respBytes, &OculusResp)
	return
}

func (m *metaApiRepositoryImpl) RequestOculusUserNonceValidate(q UserNonceValidateQuery) (OculusResp UserNonceValidateResponse, err error) {
	fullURL := fmt.Sprintf("%s%s?%s", OculusPlatformServer, UserNonceValidateUrl, q.BuildParameter())

	req, err := http.NewRequest("POST", fullURL, nil)
	if err != nil {
		return
	}

	client := http.Client{}
	if q.RequestTimeout != 0 {
		client.Timeout = q.RequestTimeout
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	if err = json.Unmarshal(respBytes, &OculusResp); err != nil {
		return
	}

	if !OculusResp.IsValid {
		err = gorm.ErrRecordNotFound
	}

	return
}

func (m *metaApiRepositoryImpl) GetOculusOrgScopedID(oculusUsrID string, q GetOculusOrgScopedIDResponseQuery) (respOrgScopedID GetOculusOrgScopedIDResponse, err error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%v/%v?%v", OculusPlatformServer, oculusUsrID, q.BuildQuery(m.AccessToken).Encode()), nil)
	if err != nil {
		return
	}

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	if err = json.Unmarshal(respBytes, &respOrgScopedID); err != nil {
		return
	}

	if !respOrgScopedID.IsValid() {
		err = gorm.ErrRecordNotFound
	}
	return
}
