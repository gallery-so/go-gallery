package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type userUpdateInput struct {
	UserNameStr string `json:"username" binding:"username"`
	BioStr      string `json:"bio"`
}

type userGetInput struct {
	UserID   persist.DBID `json:"user_id" form:"user_id"`
	Address  string       `json:"address" form:"address" binding:"eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	Username string       `json:"username" form:"username"`
}

type userGetOutput struct {
	UserID      persist.DBID `json:"id"`
	UserNameStr string       `json:"username"`
	BioStr      string       `json:"bio"`
	Addresses   []string     `json:"addresses"`
}

// INPUT - USER_CREATE - initial user creation is just an empty user, to store it in the DB.
//         this is to allow for users interupting the onboarding flow, and to be able to come back to it later
//         and the system recognize that their user already exists.
//         the users entering details on the user as they onboard are all user-update operations.
type userAddAddressInput struct {

	// needed because this is a new user that cant be logged into, and the client creating
	// the user still needs to prove ownership of their address.
	Signature string `json:"signature" binding:"required,signature"`
	Address   string `json:"address"   binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

type userCreateOutput struct {
	SignatureValid bool         `json:"signature_valid"`
	JWTtoken       string       `json:"jwt_token"` // JWT token is sent back to user to use to continue onboarding
	UserID         persist.DBID `json:"user_id"`
	GalleryID      persist.DBID `json:"gallery_id"`
}

type userAddAddressOutput struct {
	SignatureValid bool `json:"signature_valid"`
}

func updateUserInfo(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		up := &userUpdateInput{}

		if err := c.ShouldBindJSON(up); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		userID, ok := getUserIDfromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "user id not found in context"})
			return
		}

		err := userUpdateInfoDB(c, userID, up, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, successOutput{Success: true})
	}
}

func getUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &userGetInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		auth := c.GetBool(authContextKey)

		output, err := userGetDb(
			c,
			input,
			auth,
			pRuntime,
		)
		if err != nil {
			c.JSON(http.StatusNoContent, errorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, output)

	}
}

func createUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &userAddAddressInput{}

		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		output, err := userCreateDb(c, input, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, output)

	}
}
func addUserAddress(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &userAddAddressInput{}

		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		userID, ok := getUserIDfromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "user id not found in context"})
			return
		}

		output, err := addAddressToUserDB(c, userID, input, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, output)

	}
}

func userCreateDb(pCtx context.Context, pInput *userAddAddressInput,
	pRuntime *runtime.Runtime) (*userCreateOutput, error) {

	output := &userCreateOutput{}

	nonceValueStr, id, _ := getUserWithNonce(pCtx, pInput.Address, pRuntime)
	if nonceValueStr == "" {
		return nil, errors.New("nonce not found for address")
	}
	if id != "" {
		return nil, errors.New("user already exists with a given address")
	}

	sigValidBool, err := authVerifySignatureAllMethods(pInput.Signature,
		nonceValueStr,
		pInput.Address,
		pRuntime)
	if err != nil {
		return nil, err
	}

	output.SignatureValid = sigValidBool
	if !sigValidBool {
		return output, nil
	}

	user := &persist.User{
		Addresses: []string{pInput.Address},
	}

	userID, err := persist.UserCreate(pCtx, user, pRuntime)
	if err != nil {
		return nil, err
	}

	output.UserID = userID

	jwtTokenStr, err := jwtGeneratePipeline(pCtx, userID,
		pRuntime)
	if err != nil {
		return nil, err
	}

	output.JWTtoken = jwtTokenStr

	err = authNonceRotateDb(pCtx, pInput.Address, userID, pRuntime)
	if err != nil {
		return nil, err
	}

	galleryInsert := &persist.GalleryDB{OwnerUserID: userID, Collections: []persist.DBID{}}

	galleryID, err := persist.GalleryCreate(pCtx, galleryInsert, pRuntime)
	if err != nil {
		return nil, err
	}

	output.GalleryID = galleryID

	return output, nil
}

func addAddressToUserDB(pCtx context.Context, pUserID persist.DBID, pInput *userAddAddressInput,
	pRuntime *runtime.Runtime) (*userAddAddressOutput, error) {

	output := &userAddAddressOutput{}

	nonceValueStr, id, _ := getUserWithNonce(pCtx, pInput.Address, pRuntime)
	if nonceValueStr == "" {
		return nil, errors.New("nonce not found for address")
	}
	if id != "" {
		return nil, errors.New("user already exists with a given address")
	}

	dataStr := nonceValueStr
	sigValidBool, err := authVerifySignatureAllMethods(pInput.Signature,
		dataStr,
		pInput.Address,
		pRuntime)
	if err != nil {
		return nil, err
	}

	output.SignatureValid = sigValidBool
	if !sigValidBool {
		return output, nil
	}

	if err = persist.UserAddAddresses(pCtx, pUserID, []string{pInput.Address}, pRuntime); err != nil {
		return nil, err
	}

	err = authNonceRotateDb(pCtx, pInput.Address, pUserID, pRuntime)
	if err != nil {
		return nil, err
	}

	return output, nil
}

func userGetDb(pCtx context.Context, pInput *userGetInput,
	pAuthenticatedBool bool,
	pRuntime *runtime.Runtime) (*userGetOutput, error) {

	//------------------

	var user *persist.User
	var err error
	switch {
	case pInput.UserID != "":
		user, err = persist.UserGetByID(pCtx, pInput.UserID, pRuntime)
		if err != nil {
			return nil, err
		}
		break
	case pInput.Username != "":
		user, err = persist.UserGetByUsername(pCtx, pInput.Username, pRuntime)
		if err != nil {
			return nil, err
		}
		break
	case pInput.Address != "":
		user, err = persist.UserGetByAddress(pCtx, pInput.Address, pRuntime)
		if err != nil {
			return nil, err
		}
		break
	}

	if user == nil {
		return nil, errors.New("no user found")
	}

	output := &userGetOutput{
		UserID:      user.ID,
		UserNameStr: user.UserName,
		BioStr:      user.Bio,
	}

	if pAuthenticatedBool {
		output.Addresses = user.Addresses
	}

	return output, nil
}

func userUpdateInfoDB(pCtx context.Context, pUserID persist.DBID, pInput *userUpdateInput,
	pRuntime *runtime.Runtime) error {

	//------------------

	return persist.UserUpdateByID(
		pCtx,
		pUserID,
		&persist.UserUpdateInfoInput{
			UserNameIdempotent: strings.ToLower(pInput.UserNameStr),
			UserName:           pInput.UserNameStr,
			Bio:                sanitizationPolicy.Sanitize(pInput.BioStr),
		},
		pRuntime,
	)

}

// USER_DELETE__PIPELINE
func userDeleteDb(pCtx context.Context, pUserIDstr persist.DBID,
	pRuntime *runtime.Runtime) error {
	return persist.UserDelete(pCtx, pUserIDstr, pRuntime)
}

// returns nonce value string, user id
// will return empty strings and error if no nonce found
// will return empty string if no user found
func getUserWithNonce(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) (nonceValue string, userID persist.DBID, err error) {

	//------------------
	// GET_NONCE - get latest nonce for this user_address from the DB

	nonce, err := persist.AuthNonceGet(pCtx, pAddress, pRuntime)
	if err != nil {
		return nonceValue, userID, err
	}
	if nonce != nil {
		nonceValue = nonce.Value
	} else {
		return nonceValue, userID, errors.New("no nonce found")
	}

	//------------------
	// GET_ID

	user, err := persist.UserGetByAddress(pCtx, pAddress, pRuntime)
	if err != nil {
		return nonceValue, userID, err
	}
	if user != nil {
		userID = user.ID
	} else {
		return nonceValue, userID, errors.New("no user found")
	}
	//------------------

	return nonceValue, userID, nil
}
