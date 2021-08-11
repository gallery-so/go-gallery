package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
// INPUT - USER_UPDATE
type userUpdateInput struct {
	UserID      persist.DbID `json:"user_id" binding:"required"` // len=42"` // standard ETH "0x"-prefixed address
	UserNameStr string       `json:"username" binding:"username"`
	BioStr      string       `json:"description"`
	Addresses   []string     `json:"addresses"`
}

// INPUT - USER_GET
type userGetInput struct {
	UserID   persist.DbID `json:"user_id" form:"user_id"`
	Address  string       `json:"address" form:"address" binding:"eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	Username string       `json:"username" form:"username"`
}

// OUTPUT - USER_GET
type userGetOutput struct {
	UserID      persist.DbID `json:"id"`
	UserNameStr string       `json:"username"`
	BioStr      string       `json:"bio"`
	Addresses   []string     `json:"addresses"`
}

// INPUT - USER_CREATE - initial user creation is just an empty user, to store it in the DB.
//         this is to allow for users interupting the onboarding flow, and to be able to come back to it later
//         and the system recognize that their user already exists.
//         the users entering details on the user as they onboard are all user-update operations.
type userCreateInput struct {

	// needed because this is a new user that cant be logged into, and the client creating
	// the user still needs to prove ownership of their address.
	Signature string `json:"signature" binding:"required,signature"`
	Address   string `json:"address"   binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_CREATE
type userCreateOutput struct {
	SignatureValid bool         `json:"signature_valid"`
	JWTtoken       string       `json:"jwt_token"` // JWT token is sent back to user to use to continue onboarding
	UserID         persist.DbID `json:"user_id"`
}

//-------------------------------------------------------------
// HANDLERS

func updateUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		up := &userUpdateInput{}

		if err := c.ShouldBindJSON(up); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		//------------------
		// UPDATE
		err := userUpdateDb(c, up, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		//------------------
		// OUTPUT
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

		//------------------
		// OUTPUT

		c.JSON(http.StatusOK, output)

	}
}

func createUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &userCreateInput{}

		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		//------------------
		// USER_CREATE
		output, err := userCreateDb(c, input, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		//------------------
		// OUTPUT

		c.JSON(http.StatusOK, output)

	}
}

//-------------------------------------------------------------
// USER_CREATE__PIPELINE
func userCreateDb(pCtx context.Context, pInput *userCreateInput,
	pRuntime *runtime.Runtime) (*userCreateOutput, error) {

	//------------------
	output := &userCreateOutput{}

	nonceValueStr, id, _ := getUserWithNonce(pCtx, pInput.Address, pRuntime)
	if nonceValueStr == "" {
		return nil, errors.New("nonce not found for address")
	}
	if id != "" {
		return nil, errors.New("user already exists with a given address")
	}

	//------------------
	// VERIFY_SIGNATURE

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

	//------------------

	user := &persist.User{
		Addresses: []string{pInput.Address},
	}

	// DB
	userID, err := persist.UserCreate(pCtx, user, pRuntime)
	if err != nil {
		return nil, err
	}

	output.UserID = userID

	//------------------

	// JWT_GENERATION - signature is valid, so generate JWT key
	jwtTokenStr, err := jwtGeneratePipeline(pCtx, userID,
		pRuntime)
	if err != nil {
		return nil, err
	}

	output.JWTtoken = jwtTokenStr

	//------------------
	// NONCE ROTATE

	err = authNonceRotateDb(pCtx, pInput.Address, userID, pRuntime)
	if err != nil {
		return nil, err
	}

	return output, nil
}

//-------------------------------------------------------------
// USER_GET__PIPELINE
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

//-------------------------------------------------------------
func userUpdateDb(pCtx context.Context, pInput *userUpdateInput,
	pRuntime *runtime.Runtime) error {

	//------------------

	return persist.UserUpdateByID(
		pCtx,
		pInput.UserID,
		&persist.UserUpdateInput{
			UserNameIdempotent: strings.ToLower(pInput.UserNameStr),
			UserName:           pInput.UserNameStr,
			Bio:                sanitizationPolicy.Sanitize(pInput.BioStr),
			Addresses:          pInput.Addresses,
		},
		pRuntime,
	)

}

//-------------------------------------------------------------
// USER_DELETE__PIPELINE
func userDeleteDb(pCtx context.Context, pUserIDstr persist.DbID,
	pRuntime *runtime.Runtime) error {
	return persist.UserDelete(pCtx, pUserIDstr, pRuntime)
}

//-------------------------------------------------------------
// returns nonce value string, user id
// will return empty strings and error if no nonce found
// will return empty string if no user found
func getUserWithNonce(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) (nonceValue string, userID persist.DbID, err error) {

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
