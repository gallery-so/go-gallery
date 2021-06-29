package glry_lib

import (
	"context"
	"net/http"
	"os"
	"strings"

	gf_core "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
)

// information to be added to context with middlewares
type contextKey string

type authContextValue struct {
	AuthenticatedBool bool
	UserAddressStr    string
}

// jwt middleware
// parameter hell because gf_core http_handler is private :(
// both funcs (param and return funcs) are of type gf_core.http_handler implicitly
func precheckJwt(midd func(pCtx context.Context, pResp http.ResponseWriter,
	pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error), pRuntime *glry_core.Runtime) func(context.Context, http.ResponseWriter,
	*http.Request) (map[string]interface{}, *gf_core.Gf_error) {
	return func(pCtx context.Context, pResp http.ResponseWriter,
		pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

		authHeaders := strings.Split(pReq.Header.Get("Authorization"), " ")
		if len(authHeaders) > 0 {
			// get string after "Bearer"
			jwt := authHeaders[1]
			// use an env variable as jwt secret as upposed to using a stateful secret stored in
			// database that is unique to every user and session
			valid, userAddr, gErr := AuthJWTverify(jwt, os.Getenv("JWT_SECRET"), pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			// using a struct for storing values with a kard
			pCtx = context.WithValue(pCtx, contextKey("auth"), authContextValue{
				AuthenticatedBool: valid,
				UserAddressStr:    userAddr,
			})
		}
		return midd(pCtx, pResp, pReq)
	}
}

// helper func to get the current auth bool from the context
func getAuthFromCtx(pCtx context.Context) bool {
	if value, ok := pCtx.Value("auth").(authContextValue); ok {
		return value.AuthenticatedBool
	} else {
		return false
	}
}
