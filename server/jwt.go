package server

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dgrijalva/jwt-go"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	log "github.com/sirupsen/logrus"
)

// USER_JWT_KEY - is unique per user, and stored in the DB for now.
// type GLRYuserJWTkey struct {
// 	VersionInt    int64            `bson:"version"       mapstructure:"version"`
// 	ID            GLRYuserJWTkeyID `bson:"_id"           mapstructure:"_id"`
// 	CreationTimeF float64          `bson:"creation_time" mapstructure:"creation_time"`
// 	BlackListed   bool             `bson:"blacklisted"       mapstructure:"blacklisted"`

// 	ValueStr   string          `bson:"value"   mapstructure:"value"`
// 	AddressStr GLRYuserAddress `bson:"address" mapstructure:"address"`
// }

//-------------------------------------------------------------
// JWT_CLAIMS
type jwtClaims struct {
	UserId persist.DbId `json:"user_id"`
	jwt.StandardClaims
}

//-------------------------------------------------------------
// VERIFY
func authJwtParse(pJWTtokenStr string,
	pJWTsecretKeyStr string,
	pRuntime *runtime.Runtime) (bool, persist.DbId, error) {

	claims := jwtClaims{}
	JWTtoken, err := jwt.ParseWithClaims(pJWTtokenStr,
		&claims,
		func(pJWTtoken *jwt.Token) (interface{}, error) {
			return []byte(pJWTsecretKeyStr), nil
		})

	if err != nil {
		return false, "", err
	}

	log.WithFields(log.Fields{}).Debug("JWT CLAIMS --------------")
	spew.Dump(claims)

	if claims, ok := JWTtoken.Claims.(jwtClaims); ok && JWTtoken.Valid {
		return JWTtoken.Valid, claims.UserId, nil
	} else {
		return false, "", errors.New("failed to verify JWT token for a user")
	}
}

//-------------------------------------------------------------
// GENERATE__PIPELINE

// ADD!! - mark all other JWT's for this address as deleted to exclude them from future use.

func jwtGeneratePipeline(pUserId persist.DbId,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (string, error) {

	// previously we would generate a random string and use that as jwt secret and store
	// the string in the db with the jwt for verifcation. with stateless auth, we might
	// use an environment variable like so as the secret. worth considering other options
	// to increase security
	issuer := "gallery" // string(pAddressStr)
	jwtTokenStr, err := jwtGenerate(os.Getenv("JWT_SECRET"),
		issuer,
		pUserId,
		pRuntime)
	if err != nil {
		return "", err
	}

	//------------------

	return jwtTokenStr, nil
}

//-------------------------------------------------------------
// GENERATE
// ADD!! - make sure when creating new JWT tokens for user that the old ones are marked as deleted

func jwtGenerate(pSigningKeyStr string,
	pIssuerStr string,
	pUserId persist.DbId,
	pRuntime *runtime.Runtime) (string, error) {

	signingKeyBytesLst := []byte(pSigningKeyStr)

	//------------------
	// CLAIMS

	// Create the Claims
	creationTimeUNIXint := time.Now().UnixNano() / 1000000000
	expiresAtUNIXint := creationTimeUNIXint + pRuntime.Config.JWTtokenTTLsecInt //60*60*24*2 // expire N number of secs from now
	claims := jwtClaims{
		pUserId,
		jwt.StandardClaims{
			ExpiresAt: expiresAtUNIXint,
			Issuer:    pIssuerStr,
		},
	}

	//------------------

	// SYMETRIC_SIGNING - same secret is used to both sign and validate tokens
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// SIGN
	jwtTokenStr, err := token.SignedString(signingKeyBytesLst)
	if err != nil {

		return "", err
	}

	return jwtTokenStr, nil
}
