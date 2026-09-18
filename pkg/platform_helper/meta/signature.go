package meta

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func (m *metaApiRepositoryImpl) GenerateSHA256SignatureWithOculusSecret(devPayload string) string {
	h := hmac.New(sha256.New, []byte(m.AccessToken.AppSecret))
	h.Write([]byte(devPayload))
	signature := h.Sum(nil)
	return "sha256=" + hex.EncodeToString(signature)
}
