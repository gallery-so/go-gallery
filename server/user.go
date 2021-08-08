package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
// INPUT - USER_UPDATE
type userUpdateInput struct {
	UserId      persist.DbId `json:"user_id" binding:"required"` // len=42"` // standard ETH "0x"-prefixed address
	UserNameStr string       `json:"username" binding:"username"`
	BioStr      string       `json:"description"`
	Addresses   []string     `json:"addresses"`
}

// INPUT - USER_GET
type userGetInput struct {
	UserId   persist.DbId `json:"user_id" form:"user_id"`
	Address  string       `json:"address" form:"addr" binding:"eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	Username string       `json:"username" form:"username"`
}

// OUTPUT - USER_GET
type userGetOutput struct {
	UserId      persist.DbId `json:"id"`
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
	SignatureStr string `json:"signature" binding:"required,signature"`
	AddressStr   string `json:"address"   binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_CREATE
type userCreateOutput struct {
	SignatureValidBool bool         `json:"signature_valid"`
	JWTtokenStr        string       `json:"jwt_token"` // JWT token is sent back to user to use to continue onboarding
	UserIDstr          persist.DbId `json:"user_id"`
}

//-------------------------------------------------------------
// HANDLERS

func updateUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		up := &userUpdateInput{}

		if err := c.ShouldBindJSON(up); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		//------------------
		// UPDATE
		err := userUpdateDb(up, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
		//------------------
		// OUTPUT
		c.Status(http.StatusOK)
	}
}

func getUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &userGetInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		auth := c.GetBool(authContextKey)

		output, err := userGetDb(input,
			auth,
			c, pRuntime)
		if err != nil {
			c.JSON(http.StatusNoContent, ErrorResponse{Error: err.Error()})
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
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		//------------------
		// USER_CREATE
		output, err := userCreateDb(input, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}

		//------------------
		// OUTPUT

		c.JSON(http.StatusOK, output)

	}
}

//-------------------------------------------------------------
// USER_CREATE__PIPELINE
func userCreateDb(pInput *userCreateInput,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (*userCreateOutput, error) {

	//------------------
	output := &userCreateOutput{}

	nonceValueStr, id, _ := getUserWithNonce(pInput.AddressStr, pCtx, pRuntime)
	if nonceValueStr == "" {
		return nil, errors.New("nonce not found for address")
	}
	if id != "" {
		return nil, errors.New("user already exists with a given address")
	}

	//------------------
	// VERIFY_SIGNATURE

	dataStr := nonceValueStr
	sigValidBool, err := authVerifySignatureAllMethods(pInput.SignatureStr,
		dataStr,
		pInput.AddressStr,
		pRuntime)
	if err != nil {
		return nil, err
	}

	output.SignatureValidBool = sigValidBool
	if !sigValidBool {
		return output, nil
	}

	//------------------

	user := &persist.User{
		AddressesLst: []string{pInput.AddressStr},
	}

	// DB
	userId, err := persist.UserCreate(user, pCtx, pRuntime)
	if err != nil {
		return nil, err
	}

	output.UserIDstr = userId

	//------------------

	// JWT_GENERATION - signature is valid, so generate JWT key
	jwtTokenStr, err := jwtGeneratePipeline(userId,
		pCtx,
		pRuntime)
	if err != nil {
		return nil, err
	}

	output.JWTtokenStr = jwtTokenStr

	//------------------
	// NONCE ROTATE

	err = authNonceRotateDb(pInput.AddressStr, userId, pCtx, pRuntime)
	if err != nil {
		return nil, err
	}

	return output, nil
}

//-------------------------------------------------------------
// USER_GET__PIPELINE
func userGetDb(pInput *userGetInput,
	pAuthenticatedBool bool,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (*userGetOutput, error) {

	//------------------

	var user *persist.User
	var err error
	switch {
	case pInput.UserId != "":
		user, err = persist.UserGetById(pInput.UserId, pCtx, pRuntime)
		if err != nil {
			return nil, err
		}
		break
	case pInput.Username != "":
		user, err = persist.UserGetByUsername(pInput.Username, pCtx, pRuntime)
		if err != nil {
			return nil, err
		}
		break
	case pInput.Address != "":
		user, err = persist.UserGetByAddress(pInput.Address, pCtx, pRuntime)
		if err != nil {
			return nil, err
		}
		break
	}

	if user == nil {
		return nil, errors.New("no user found")
	}

	output := &userGetOutput{
		UserId:      user.IDstr,
		UserNameStr: user.UserNameStr,
		BioStr:      user.BioStr,
	}

	if pAuthenticatedBool {
		output.Addresses = user.AddressesLst
	}

	return output, nil
}

//-------------------------------------------------------------
func userUpdateDb(pInput *userUpdateInput,
	pCtx context.Context,
	pRuntime *runtime.Runtime) error {

	//------------------

	return persist.UserUpdateById(
		pInput.UserId,
		persist.UserUpdateInput{
			UserNameStr:  pInput.UserNameStr,
			BioStr:       sanitizationPolicy.Sanitize(pInput.BioStr),
			AddressesLst: pInput.Addresses,
		},
		pCtx,
		pRuntime,
	)

}

//-------------------------------------------------------------
// USER_DELETE__PIPELINE
func userDeleteDb(pUserIDstr persist.DbId,
	pCtx context.Context,
	pRuntime *runtime.Runtime) error {
	return persist.UserDelete(pUserIDstr, pCtx, pRuntime)
}

//-------------------------------------------------------------
// returns nonce value string, user id
// will return empty strings and error if no nonce found
// will return empty string if no user found
func getUserWithNonce(pAddress string,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (nonceValueStr string, userIdStr persist.DbId, err error) {

	//------------------
	// GET_NONCE - get latest nonce for this user_address from the DB

	nonce, err := persist.AuthNonceGet(pAddress,
		pCtx,
		pRuntime)
	if err != nil {
		return nonceValueStr, userIdStr, err
	}
	if nonce != nil {
		nonceValueStr = nonce.ValueStr
	} else {
		return nonceValueStr, userIdStr, errors.New("no nonce found")
	}

	//------------------
	// GET_ID

	user, err := persist.UserGetByAddress(pAddress, pCtx, pRuntime)
	if err != nil {
		return nonceValueStr, userIdStr, err
	}
	if user != nil {
		userIdStr = user.IDstr
	} else {
		return nonceValueStr, userIdStr, errors.New("no user found")
	}
	//------------------

	return nonceValueStr, userIdStr, nil
}
