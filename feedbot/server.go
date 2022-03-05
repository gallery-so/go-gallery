package feedbot

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
)

type Event struct {
	ID   persist.DBID `json:"id" binding:"required"`
	Type string       `json:"event_type" binding:"required"`
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

func ping() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ping": "pong"})
	}
}

func eventNewUser(eventRepo *postgres.EventRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := EventUserCreated{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
		}
		c.JSON(http.StatusOK, gin.H{"msg": "task accepted"})
	}
}

func eventUpdateNFT(eventRepo *postgres.EventRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := EventNFTUpdated{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
		}
		c.JSON(http.StatusOK, gin.H{"msg": "task accepted"})
	}
}

func eventNewCollection(eventRepo *postgres.EventRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := EventNewCollection{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
		}
		c.JSON(http.StatusOK, gin.H{"msg": "task accepted"})
	}
}

func eventUpdateCollectionInfo(eventRepo *postgres.EventRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := EventUpdateCollectionInfo{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
		}
		c.JSON(http.StatusOK, gin.H{"msg": "task accepted"})
	}
}

func eventUpdateCollectionNFTs(eventRepo *postgres.EventRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := EventUpdateCollectionNFTs{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
		}
		c.JSON(http.StatusOK, gin.H{"msg": "task accepted"})
	}
}
