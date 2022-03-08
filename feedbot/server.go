package feedbot

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/event/cloudtask"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

func handleMessage(userRepo persist.UserRepository, userEventRepo persist.UserEventRepository, tokenEventRepo persist.NftEventRepository, collectionEventRepo persist.CollectionEventRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := cloudtask.EventMessage{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		switch persist.CategoryFromEventCode(input.EventCode) {
		case persist.UserEventCode:
			err := handleUserEvents(ctx, userRepo, userEventRepo, input)
			if err != nil {
				if err == errInvalidUserEvent || err == errMissingUserEvent {
					util.ErrResponse(c, http.StatusOK, err)
					return
				} else {
					util.ErrResponse(c, http.StatusInternalServerError, err)
					return
				}
			}
		case persist.NftEventCode:
			err := handleNftEvents(ctx, userRepo, tokenEventRepo, input)
			if err != nil {
				if err == errInvalidNftEvent || err == errMissingNftEvent {
					util.ErrResponse(c, http.StatusOK, err)
					return
				} else {
					util.ErrResponse(c, http.StatusInternalServerError, err)
				}
			}
		case persist.CollectionEventCode:
			err := handleCollectionEvents(ctx, userRepo, collectionEventRepo, input)
			if err != nil {
				if err == errInvalidCollectionEvent || err == errMissingCollectionEvent {
					util.ErrResponse(c, http.StatusOK, err)
					return
				} else {
					util.ErrResponse(c, http.StatusInternalServerError, err)
				}
			}
		default:
			util.ErrResponse(c, http.StatusOK, errors.New("unknown event type"))
			return
		}

		c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("event(%s) processed", input.ID)})
	}
}

func ping() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ping": "pong"})
	}
}
