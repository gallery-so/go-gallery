package middleware

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// RequiredNFTs is a list of NFTs that are required for the user to be able to use the service as an authenticated user
var RequiredNFTs = []persist.TokenID{"0", "1", "2", "3", "4", "5", "6", "7", "8"}

const (
	// UserIDContextKey is the key for the user id in the context
	UserIDContextKey = "user_id"
	// AuthContextKey is the key for the auth status in the context
	AuthContextKey = "authenticated"
)

var rateLimiter = newIPRateLimiter(1, 5)

// ErrInvalidJWT is returned when the JWT is invalid
var ErrInvalidJWT = errors.New("invalid JWT")

// ErrRateLimited is returned when the IP address has exceeded the rate limit
var ErrRateLimited = errors.New("rate limited")

// ErrInvalidAuthHeader is returned when the auth header is invalid
var ErrInvalidAuthHeader = errors.New("invalid auth header format")

var mixpanelDistinctIDs = map[string]string{}

type errUserDoesNotHaveRequiredNFT struct {
	addresses []persist.Address
}

// JWTRequired is a middleware that checks if the user is authenticated
func JWTRequired(userRepository persist.UserRepository, ethClient *eth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: ErrInvalidAuthHeader.Error()})
			return
		}
		authHeaders := strings.Split(header, " ")
		if len(authHeaders) == 2 {
			if authHeaders[0] == viper.GetString("ADMIN_PASS") {
				c.Set(UserIDContextKey, persist.DBID(authHeaders[1]))
				c.Next()
				return
			}
			if authHeaders[0] != "Bearer" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: ErrInvalidAuthHeader.Error()})
				return
			}
			// get string after "Bearer"
			jwt := authHeaders[1]
			// use an env variable as jwt secret as upposed to using a stateful secret stored in
			// database that is unique to every user and session
			valid, userID, err := AuthJWTParse(jwt, viper.GetString("JWT_SECRET"))
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: err.Error()})
				return
			}

			if !valid {
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: ErrInvalidJWT.Error()})
				return
			}

			if viper.GetBool("REQUIRE_NFTS") {
				user, err := userRepository.GetByID(c, userID)
				if err != nil {
					c.AbortWithStatusJSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
					return
				}
				has := false
				for _, addr := range user.Addresses {
					if res, _ := ethClient.HasNFTs(c, RequiredNFTs, addr); res {
						has = true
						break
					}
				}
				if !has {
					c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserDoesNotHaveRequiredNFT{addresses: user.Addresses}.Error()})
					return
				}
			}

			c.Set(UserIDContextKey, userID)
		} else {
			c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: ErrInvalidAuthHeader.Error()})
			return
		}
		c.Next()
	}
}

// JWTOptional is a middleware that checks if the user is authenticated and if so stores
// auth data in the context
func JWTOptional() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header != "" {
			authHeaders := strings.Split(header, " ")
			if len(authHeaders) == 2 {
				if authHeaders[0] == viper.GetString("ADMIN_PASS") {
					c.Set(UserIDContextKey, persist.DBID(authHeaders[1]))
					c.Next()
					return
				}
				// get string after "Bearer"
				jwt := authHeaders[1]
				valid, userID, _ := AuthJWTParse(jwt, viper.GetString("JWT_SECRET"))
				c.Set(AuthContextKey, valid)
				c.Set(UserIDContextKey, userID)
			} else {
				c.Set(AuthContextKey, false)
				c.Set(UserIDContextKey, persist.DBID(""))
			}
		} else {
			c.Set(AuthContextKey, false)
			c.Set(UserIDContextKey, persist.DBID(""))
		}
		c.Next()
	}
}

// RateLimited is a middleware that rate limits requests by IP address
func RateLimited() gin.HandlerFunc {
	return func(c *gin.Context) {
		limiter := rateLimiter.getLimiter(c.ClientIP())
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: ErrRateLimited.Error()})
			return
		}
		c.Next()
	}
}

// HandleCORS sets the CORS headers
func HandleCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestOrigin := c.Request.Header.Get("Origin")
		allowedOrigins := strings.Split(viper.GetString("ALLOWED_ORIGINS"), ",")

		if util.Contains(allowedOrigins, requestOrigin) || (strings.ToLower(viper.GetString("ENV")) == "development" && strings.HasSuffix(requestOrigin, "-gallery-so.vercel.app")) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", requestOrigin)
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// MixpanelTrack is a middleware that tracks events in MixPanel
func MixpanelTrack(eventName string, keys []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		var distinctID string
		userID, ok := c.Get(UserIDContextKey)
		if ok {
			distinctID = string(userID.(persist.DBID))
		} else {
			uniqueID, ok := mixpanelDistinctIDs[c.ClientIP()]
			if !ok {
				distinctID = uuid.New().String()
				mixpanelDistinctIDs[c.ClientIP()] = distinctID
			} else {
				distinctID = uniqueID
			}
		}

		vals := url.Values{}
		data := map[string]interface{}{
			"event": eventName,
			"properties": map[string]interface{}{
				"distinct_id": distinctID,
				"ip":          c.ClientIP(),
				"params":      c.Params,
			},
		}
		if keys != nil {
			for _, key := range keys {
				val := c.Value(key)
				if val != nil {
					data["properties"].(map[string]interface{})[key] = val
				}
			}
		}
		marshalled, err := json.Marshal(data)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"Data": vals, "DistinctID": distinctID, "EventName": eventName}).Error("error tracking mixpanel event")
			return
		}
		vals.Set("data", string(marshalled))
		payload := strings.NewReader(vals.Encode())
		req, err := http.NewRequest(http.MethodPost, viper.GetString("MIXPANEL_API_URL"), payload)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"Data": vals, "DistinctID": distinctID, "EventName": eventName}).Error("error tracking mixpanel event")
			return
		}

		req.Header.Add("Accept", "text/plain")
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"Data": vals, "DistinctID": distinctID, "EventName": eventName}).Error("error tracking mixpanel event")
			return
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			logrus.WithFields(logrus.Fields{"Data": vals, "DistinctID": distinctID, "EventName": eventName, "Status": res.Status}).Error("error tracking mixpanel event")
			return
		}

	}
}

// ErrLogger is a middleware that logs errors
func ErrLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 {
			logrus.Errorf("%s %s %s %s %s", c.Request.Method, c.Request.URL, c.ClientIP(), c.Request.Header.Get("User-Agent"), c.Errors.JSON())
		}
	}
}

// GetUserIDFromCtx returns the user ID from the context
func GetUserIDFromCtx(c *gin.Context) persist.DBID {
	return c.MustGet(UserIDContextKey).(persist.DBID)
}

func (e errUserDoesNotHaveRequiredNFT) Error() string {
	return fmt.Sprintf("required tokens not owned by addresses: %v", e.addresses)
}
