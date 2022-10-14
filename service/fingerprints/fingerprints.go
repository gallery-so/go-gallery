package fingerprints

import (
	"errors"

	"github.com/gin-gonic/gin"
)

const fingerprintContextKey = "fingerprints.fingerprint"

const fingerprintCookieKey = "GLRY_FINGERPRINT"

var ErrNoFingerprint = errors.New("no fingerprint found")

type Fingerprint string

func (f Fingerprint) String() string {
	return string(f)
}

func (f Fingerprint) Value() (interface{}, error) {
	return f.String(), nil
}

func (f *Fingerprint) Scan(src interface{}) error {
	if src == nil {
		return ErrNoFingerprint
	}

	switch src := src.(type) {
	case string:
		*f = Fingerprint(src)
		return nil
	default:
		return ErrNoFingerprint
	}
}

func GetFingerprintFromCtx(c *gin.Context) (Fingerprint, error) {
	fp, ok := c.Get(fingerprintContextKey)
	if !ok {
		cookie, err := c.Cookie(fingerprintCookieKey)
		if err != nil {
			return "", ErrNoFingerprint
		}

		c.Set(fingerprintContextKey, cookie)

		return Fingerprint(cookie), nil
	}
	return fp.(Fingerprint), nil
}

func ContainsFingerprint(fpts []Fingerprint, fpt Fingerprint) bool {
	for _, f := range fpts {
		if f == fpt {
			return true
		}
	}
	return false
}
