package meta

import (
	"fmt"
	"log"
	"sync"
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
	PlatformServer string
	AppID          string
	AppSecret      string

	once        sync.Once
	accessToken string
}

func (c *OCULUSPlatformConfig) FormAccessToken() string {
	c.once.Do(func() {
		c.accessToken = fmt.Sprintf("OC|%v|%v", c.AppID, c.AppSecret)
		log.Printf("FormAccessToken using %v %v: %v", c.AppID, c.AppSecret, c.accessToken)
	})
	return c.accessToken
}

