package auth

import (
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

const authContextName = "auth context"

func SetAuthContext(scope *sentry.Scope, gc *gin.Context) {
	var authCtx sentry.Context
	var userCtx sentry.User

	if GetUserAuthedFromCtx(gc) {
		userID := string(GetUserIDFromCtx(gc))
		sessionID := string(GetSessionIDFromCtx(gc))
		authCtx = sentry.Context{
			"Authenticated": true,
			"UserID":        userID,
			"SessionID":     sessionID,
		}
		userCtx = sentry.User{ID: userID}
	} else {
		authCtx = sentry.Context{
			"AuthError": GetAuthErrorFromCtx(gc),
		}
		userCtx = sentry.User{}
	}

	scope.SetContext(authContextName, authCtx)
	scope.SetUser(userCtx)
}

func ScrubEventCookies(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if event == nil || event.Request == nil {
		return event
	}

	var scrubbed []string
	for _, c := range strings.Split(event.Request.Cookies, "; ") {
		if strings.HasPrefix(c, AuthCookieKey) {
			scrubbed = append(scrubbed, AuthCookieKey+"=[filtered]")
		} else if strings.HasPrefix(c, RefreshCookieKey) {
			scrubbed = append(scrubbed, RefreshCookieKey+"=[filtered]")
		} else {
			scrubbed = append(scrubbed, c)
		}
	}
	cookies := strings.Join(scrubbed, "; ")

	event.Request.Cookies = cookies
	event.Request.Headers["Cookie"] = cookies
	return event
}
