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
	UserId      persist.DbId `json:"address" binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	UserNameStr string       `json:"username"`
	BioStr      string       `json:"description"`
}

// INPUT - USER_GET
type userGetInput struct {
	UserId   persist.DbId `json:"user_id" form:"user_id"`
	Address  string       `json:"address" form:"addr" binding:"eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	Username string       `json:"username" form:"username"`
}

// OUTPUT - USER_GET
type userGetOutput struct {
	UserNameStr string   ` json:"username"`
	BioStr      string   ` json:"bio"`
	Addresses   []string ` json:"addresses"`
}

// INPUT - USER_CREATE - initial user creation is just an empty user, to store it in the DB.
//         this is to allow for users interupting the onboarding flow, and to be able to come back to it later
//         and the system recognize that their user already exists.
//         the users entering details on the user as they onboard are all user-update operations.
type userCreateInput struct {

	// needed because this is a new user that cant be logged into, and the client creating
	// the user still needs to prove ownership of their address.
	SignatureStr  string `json:"signature" binding:"required,signature"`
	AddressStr    string `json:"address"   binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	NonceValueStr string `json:"nonce"     binding:"required,nonce"`
}

// OUTPUT - USER_CREATE
type userCreateOutput struct {
	SignatureValidBool bool         `json:"signature_valid"`
	JWTtokenStr        string       `json:"jwt_token"` // JWT token is sent back to user to use to continue onboarding
	UserIDstr          persist.DbId `json:"user_id"`
}

//-------------------------------------------------------------
// HANDLERS

func updateUserAuth(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		if auth := c.GetBool("authenticated"); !auth {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization required"})
			return
		}

		up := &userUpdateInput{}

		if err := c.ShouldBindJSON(up); err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error()})
			return
		}

		//------------------
		// UPDATE
		gErr := userUpdateDb(up, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusOK, gin.H{"error": gErr})
			return
		}
		//------------------
		// OUTPUT
		c.Status(http.StatusOK)
	}
}

func getUserAuth(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		auth := c.GetBool("authenticated")

		input := &userGetInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error()})
			return
		}

		output, err := userGetDb(input,
			auth,
			c, pRuntime)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error()})
			return
		}

		//------------------
		// OUTPUT

		c.JSON(http.StatusOK, output)

	}
}

func createUserAuth(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &userCreateInput{}

		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error()})
			return
		}

		//------------------
		// USER_CREATE
		output, gErr := userCreateDb(input, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusOK, gin.H{"error": gErr})
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
	// VALIDATE
	err := runtime.Validate(pInput, pRuntime)
	if err != nil {
		return nil, err
	}

	//------------------
	output := &userCreateOutput{}

	//------------------
	// USER_CHECK
	// _, nonceValueStr, _, gErr := authUserCheck(pInput, pCtx, pRuntime)
	// if gErr != nil {
	// 	return nil, gErr
	// }

	//------------------
	// VERIFY_SIGNATURE

	dataStr := pInput.NonceValueStr
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
	id, err := persist.UserCreate(user, pCtx, pRuntime)
	if err != nil {
		return nil, err
	}

	output.UserIDstr = id

	//------------------

	// JWT_GENERATION - signature is valid, so generate JWT key
	jwtTokenStr, gErr := jwtGeneratePipeline(id,
		pCtx,
		pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	output.JWTtokenStr = jwtTokenStr

	//------------------

	return output, nil
}

//-------------------------------------------------------------
// USER_GET__PIPELINE
func userGetDb(pInput *userGetInput,
	pAuthenticatedBool bool,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (*userGetOutput, error) {

	//------------------
	// VALIDATE
	err := runtime.Validate(pInput, pRuntime)
	if err != nil {
		return nil, err
	}

	//------------------

	var user *persist.User
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

	output := &userGetOutput{}
	if pAuthenticatedBool {
		output = &userGetOutput{
			UserNameStr: user.UserNameStr,
			BioStr:      user.BioStr,
			Addresses:   user.AddressesLst,
		}
	} else {
		// TODO
	}

	return output, nil
}

//-------------------------------------------------------------
func userUpdateDb(pInput *userUpdateInput,
	pCtx context.Context,
	pRuntime *runtime.Runtime) error {

	//------------------
	// VALIDATE
	err := runtime.Validate(pInput, pRuntime)
	if err != nil {
		return err
	}

	//------------------

	return persist.UserUpdate(
		&persist.User{
			IDstr:       pInput.UserId,
			UserNameStr: pInput.UserNameStr,
			BioStr:      pInput.BioStr,
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
func userIsValid(pAddress string,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (bool, string, persist.DbId, error) {

	//------------------
	// CHECK_USER_EXISTS
	userExistsBool, err := persist.UserExistsByAddress(pAddress,
		pCtx,
		pRuntime)
	if err != nil {
		return false, "", "", err
	}

	//------------------
	// GET_NONCE - get latest nonce for this user_address from the DB

	nonce, err := persist.AuthNonceGet(pAddress,
		pCtx,
		pRuntime)
	if err != nil {
		return false, "", "", err
	}

	// NONCE_NOT_FOUND - for this particular user
	var nonceValueStr string
	if nonce == nil {
		nonceValueStr = ""
	} else {
		nonceValueStr = nonce.ValueStr
	}

	//------------------
	// GET_ID

	var userIDstr persist.DbId
	if userExistsBool {

		user, err := persist.UserGetByAddress(pAddress, pCtx, pRuntime)
		if err != nil {
			return false, "", "", err
		}

		userIDstr = user.IDstr
	}

	//------------------

	return userExistsBool, nonceValueStr, userIDstr, nil
}
