package user

type DevicePlatform string

const (
	DevicePlatformWeb   DevicePlatform = "web"   // account/password
	DevicePlatformEMAIL DevicePlatform = "email" // account with email TOTP

	DevicePlatformGOOGLE DevicePlatform = "google"
	DevicePlatformAPPLE  DevicePlatform = "apple"
	DevicePlatformOCULUS DevicePlatform = "oculus"
	DevicePlatformSTEAM  DevicePlatform = "steam"
	DevicePlatformPICO   DevicePlatform = "pico"

	DevicePlatformWORKER DevicePlatform = "worker" // reserved, can only be created by ADMIN
)

var (
	DevicePlatformList = []DevicePlatform{
		DevicePlatformWeb,
		DevicePlatformEMAIL,
		DevicePlatformGOOGLE,
		DevicePlatformAPPLE,
		DevicePlatformOCULUS,
		DevicePlatformSTEAM,
		DevicePlatformPICO,
	}

	DevicePlatformOAuthList = []DevicePlatform{
		DevicePlatformGOOGLE,
		DevicePlatformAPPLE,
		DevicePlatformOCULUS,
		DevicePlatformSTEAM,
		DevicePlatformPICO,
	}

	DevicePlatformAcronym = map[DevicePlatform]string{
		DevicePlatformWeb:    "w",
		DevicePlatformEMAIL:  "e",
		DevicePlatformGOOGLE: "g",
		DevicePlatformAPPLE:  "a",
		DevicePlatformOCULUS: "o",
		DevicePlatformSTEAM:  "s",
		DevicePlatformPICO:   "p",
		DevicePlatformWORKER: "w",
	}

	AcronymToDevicePlatform = map[string]DevicePlatform{
		DevicePlatformAcronym[DevicePlatformWeb]:    DevicePlatformWeb,
		DevicePlatformAcronym[DevicePlatformEMAIL]:  DevicePlatformEMAIL,
		DevicePlatformAcronym[DevicePlatformGOOGLE]: DevicePlatformGOOGLE,
		DevicePlatformAcronym[DevicePlatformAPPLE]:  DevicePlatformAPPLE,
		DevicePlatformAcronym[DevicePlatformOCULUS]: DevicePlatformOCULUS,
		DevicePlatformAcronym[DevicePlatformSTEAM]:  DevicePlatformSTEAM,
		DevicePlatformAcronym[DevicePlatformPICO]:   DevicePlatformPICO,
		DevicePlatformAcronym[DevicePlatformWORKER]: DevicePlatformWORKER,
	}
)
