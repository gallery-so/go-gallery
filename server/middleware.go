package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mikeydub/go-gallery/eth"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	userIDcontextKey = "user_id"
	authContextKey   = "authenticated"
)

var rateLimiter = newIPRateLimiter(1, 5)

var errInvalidJWT = errors.New("invalid JWT")

var errRateLimited = errors.New("rate limited")

var errInvalidAuthHeader = errors.New("invalid auth header format")

var mixpanelDistinctIDs = map[string]string{}

type errUserDoesNotHaveRequiredNFT struct {
	addresses []persist.Address
}

func jwtRequired(userRepository persist.UserRepository, ethClient *eth.Client, tokenIDs []persist.TokenID) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: errInvalidAuthHeader.Error()})
			return
		}
		authHeaders := strings.Split(header, " ")
		if len(authHeaders) == 2 {
			if authHeaders[0] == viper.GetString("ADMIN_PASS") {
				c.Set(userIDcontextKey, persist.DBID(authHeaders[1]))
				c.Next()
				return
			}
			if authHeaders[0] != "Bearer" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: errInvalidAuthHeader.Error()})
				return
			}
			// get string after "Bearer"
			jwt := authHeaders[1]
			// use an env variable as jwt secret as upposed to using a stateful secret stored in
			// database that is unique to every user and session
			valid, userID, err := authJwtParse(jwt, viper.GetString("JWT_SECRET"))
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: err.Error()})
				return
			}

			if !valid {
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: errInvalidJWT.Error()})
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
					if res, _ := ethClient.HasNFTs(c, tokenIDs, addr); res {
						has = true
						break
					}
				}
				if !has {
					c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserDoesNotHaveRequiredNFT{addresses: user.Addresses}.Error()})
					return
				}
			}

			c.Set(userIDcontextKey, userID)
		} else {
			c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: errInvalidAuthHeader.Error()})
			return
		}
		c.Next()
	}
}

func jwtOptional() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header != "" {
			authHeaders := strings.Split(header, " ")
			if len(authHeaders) == 2 {
				if authHeaders[0] == viper.GetString("ADMIN_PASS") {
					c.Set(userIDcontextKey, persist.DBID(authHeaders[1]))
					c.Next()
					return
				}
				// get string after "Bearer"
				jwt := authHeaders[1]
				valid, userID, _ := authJwtParse(jwt, viper.GetString("JWT_SECRET"))
				c.Set(authContextKey, valid)
				c.Set(userIDcontextKey, userID)
			} else {
				c.Set(authContextKey, false)
				c.Set(userIDcontextKey, persist.DBID(""))
			}
		} else {
			c.Set(authContextKey, false)
			c.Set(userIDcontextKey, persist.DBID(""))
		}
		c.Next()
	}
}

func rateLimited() gin.HandlerFunc {
	return func(c *gin.Context) {
		limiter := rateLimiter.getLimiter(c.ClientIP())
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: errRateLimited.Error()})
			return
		}
		c.Next()
	}
}

func handleCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestOrigin := c.Request.Header.Get("Origin")
		allowedOrigins := strings.Split(viper.GetString("ALLOWED_ORIGINS"), ",")

		if util.Contains(allowedOrigins, requestOrigin) || (strings.ToLower(viper.GetString("ENV")) == "development" && strings.HasPrefix(requestOrigin, "https://gallery-git-") && strings.HasSuffix(requestOrigin, "-gallery-so.vercel.app")) {
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

func mixpanelTrack(eventName string, keys []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		var distinctID string
		userID, ok := c.Get(userIDcontextKey)
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

func errLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 {
			logrus.Errorf("%s %s %s %s %s", c.Request.Method, c.Request.URL, c.ClientIP(), c.Request.Header.Get("User-Agent"), c.Errors.JSON())
		}
	}
}

func getUserIDfromCtx(c *gin.Context) persist.DBID {
	return c.MustGet(userIDcontextKey).(persist.DBID)
}

func (e errUserDoesNotHaveRequiredNFT) Error() string {
	return fmt.Sprintf("required tokens not owned by addresses: %v", e.addresses)
}
