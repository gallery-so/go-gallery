package feedbot

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
)

const (
	eventTypeUserCreated          persist.EventType = "userCreated"
	eventTypeUpdateNFT            persist.EventType = "updateNFT"
	eventTypeNewCollection        persist.EventType = "newCollection"
	eventTypeUpdateCollectionInfo persist.EventType = "updateCollectionInfo"
	eventTypeUpdateCollectionNFTs persist.EventType = "updateCollectionNFTs"
)

type Event struct {
	ID      persist.DBID      `json:"id" binding:"required"`
	Type    persist.EventType `json:"event_type" binding:"required"`
	Message string            `json:"message" binding:"required"`
}

type EventUserCreated struct {
	Event
	UserID   persist.DBID `json:"user_id" binding:"required"`
	Username string       `json:"message" binding:"required"`
}

type EventNFTUpdated struct {
	Event
	UserID         persist.DBID       `json:"user_id" binding:"required"`
	NftID          persist.DBID       `json:"id" binding:"required"`
	CollectorsNote persist.NullString `json:"collectors_note"`
}

type EventNewCollection struct {
	Event
	UserID       persist.DBID `json:"user_id" binding:"required"`
	CollectionID persist.DBID `json:"collection_id" binding:"required"`
}

type EventUpdateCollectionInfo struct {
	Event
	UserID       persist.DBID `json:"user_id" binding:"required"`
	CollectionID persist.DBID `json:"collection_id" binding:"required"`
}

type EventUpdateCollectionNFTs struct {
	Event
	UserID       persist.DBID                `json:"user_id" binding:"required"`
	CollectionID persist.DBID                `json:"collection_id" binding:"required"`
	NFTs         []persist.TokenInCollection `json:"nfts" binding:"required"`
}

type EventToRoute map[persist.EventType]func(*gin.Context, *postgres.EventRepository, Event)

func ping() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ping": "pong"})
	}
}

func handleEvent(eventRepo *postgres.EventRepository, routes EventToRoute) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := Event{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
		}

		handle, exists := routes[input.Type]
		if !exists {
			util.ErrResponse(c, http.StatusBadRequest, errors.New("unknown event type"))
		}

		handle(c, eventRepo, input)
	}
}

func handleEventNewUser(c *gin.Context, eventRepo *postgres.EventRepository, event Event) {
	message := EventUserCreated{}
	if err := util.UnmarshallBody(&message, strings.NewReader(event.Message)); err != nil {
		util.ErrResponse(c, http.StatusBadRequest, err)
	}
	c.JSON(http.StatusOK, gin.H{"msg": "task accepted"})
}

func handleEventUpdateNFT(c *gin.Context, eventRepo *postgres.EventRepository, event Event) {
	message := EventNFTUpdated{}
	if err := util.UnmarshallBody(&message, strings.NewReader(event.Message)); err != nil {
		util.ErrResponse(c, http.StatusBadRequest, err)
	}
	c.JSON(http.StatusOK, gin.H{"msg": "task accepted"})
}

func handleEventNewCollection(c *gin.Context, eventRepo *postgres.EventRepository, event Event) {
	message := EventNewCollection{}
	if err := util.UnmarshallBody(&message, strings.NewReader(event.Message)); err != nil {
		util.ErrResponse(c, http.StatusBadRequest, err)
	}
	c.JSON(http.StatusOK, gin.H{"msg": "task accepted"})
}

func handleEventUpdateCollectionInfo(c *gin.Context, eventRepo *postgres.EventRepository, event Event) {
	message := EventUpdateCollectionInfo{}
	if err := util.UnmarshallBody(&message, strings.NewReader(event.Message)); err != nil {
		util.ErrResponse(c, http.StatusBadRequest, err)
	}
	c.JSON(http.StatusOK, gin.H{"msg": "task accepted"})
}

func handleEventUpdateCollectionNFTs(c *gin.Context, eventRepo *postgres.EventRepository, event Event) {
	message := EventUpdateCollectionNFTs{}
	if err := util.UnmarshallBody(&message, strings.NewReader(event.Message)); err != nil {
		util.ErrResponse(c, http.StatusBadRequest, err)
	}
}
