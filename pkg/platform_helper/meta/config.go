package meta

import (
	"fmt"
	"log"
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

type OCULUSPlatformConfig struct {
	AppID     string
	AppSecret string
}

func (c *OCULUSPlatformConfig) FormAccessToken() (oculusPlatformAccessToken string) {
	oculusPlatformAccessToken = fmt.Sprintf("OC|%v|%v", c.AppID, c.AppSecret)
	log.Printf("FormAccessToken using %v %v: %v", c.AppID, c.AppSecret, oculusPlatformAccessToken)
	return
}
