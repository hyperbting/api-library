package totphelper

import (
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"
	"math"
	"time"
)

var myValidateOpts = totp.ValidateOpts{
	Period:    30, // default 30 seconds WINDOW
	Skew:      10, // Periods before or after the current time to allow.  Skew 5 means  5+1+5 for period 20s it will be 2.5min + 0.5min + 2.5min
	Digits:    otp.DigitsSix,
	Algorithm: otp.AlgorithmSHA1,
}

// BuildTOTP generate
func BuildTOTP(uID string) (key *otp.Key, err error) {

	key, err = totp.Generate(totp.GenerateOpts{
		Issuer:      "Hooloop.com",            // Name of the issuing Organization/Company.
		AccountName: uID,                      // Name of the User's Account (eg, email address)
		SecretSize:  32,                       // Size in size of the generated Secret. Defaults to 10 bytes.
		Period:      myValidateOpts.Period,    // Number of seconds a TOTP hash is valid for. Defaults to 30 seconds.
		Digits:      myValidateOpts.Digits,    // Digits to request. Defaults to 6.
		Algorithm:   myValidateOpts.Algorithm, // Algorithm to use for HMAC. Defaults to SHA1.
	})
	if err != nil {
		panic(err)
	}
	//log.Print(key.AccountName)
	return key, err
}

// VerifyPassCode is used to verify pass code
func VerifyPassCode(passcode string, keySecret string) (valid bool) {
	valid, _ = totp.ValidateCustom(
		passcode,
		keySecret,
		time.Now().UTC(),
		myValidateOpts,
	)
	return valid
}

// GeneratePassCode generate passcode
func GeneratePassCode(secret string) (string, error) {
	return totp.GenerateCodeCustom(
		secret,
		time.Now().UTC(),
		myValidateOpts,
	)
}

// GenerateTOTPCodes generate passcode
func GenerateTOTPCodes(secret string) []string {

	var counter = int64(math.Floor(float64(time.Now().Unix()) / float64(myValidateOpts.Period)))

	passcode0, _ := hotp.GenerateCodeCustom(secret, uint64(counter), hotp.ValidateOpts{
		Digits:    myValidateOpts.Digits,
		Algorithm: myValidateOpts.Algorithm,
	})

	result := []string{passcode0}

	for i := int64(1); i < int64(myValidateOpts.Skew); i++ {
		passcode, _ := hotp.GenerateCodeCustom(secret, uint64(counter+i), hotp.ValidateOpts{
			Digits:    myValidateOpts.Digits,
			Algorithm: myValidateOpts.Algorithm,
		})
		passcode2, _ := hotp.GenerateCodeCustom(secret, uint64(counter-i), hotp.ValidateOpts{
			Digits:    myValidateOpts.Digits,
			Algorithm: myValidateOpts.Algorithm,
		})
		result = append(result, passcode)
		result = append(result, passcode2)
	}

	return result
}
