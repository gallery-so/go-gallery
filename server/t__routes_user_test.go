package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/stretchr/testify/assert"
)

func TestGetUserByID_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with user
	username := "BingBong"
	userID, err := persist.UserCreate(context.Background(), &persist.User{
		UserName:           username,
		UserNameIdempotent: strings.ToLower(username),
		Addresses:          []string{tc.user1.address},
		Bio:                "punk",
	}, tc.r)
	assert.Nil(err)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?user_id=%s", tc.serverURL, userID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Equal(username, body.UserName)
}

func TestGetUserByAddress_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with user
	username := "BongBing"
	_, err := persist.UserCreate(context.Background(), &persist.User{
		UserName:           username,
		UserNameIdempotent: strings.ToLower(username),
		Addresses:          []string{tc.user2.address},
		Bio:                "punk",
	}, tc.r)
	assert.Nil(err)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?address=%s", tc.serverURL, tc.user2.address))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Equal(username, body.UserName)
}

func TestGetUserByUsername_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with user
	username := "BingBongBing"
	_, err := persist.UserCreate(context.Background(), &persist.User{
		UserName:           username,
		UserNameIdempotent: strings.ToLower(username),
		Addresses:          []string{tc.user1.address},
		Bio:                "punk",
	}, tc.r)
	assert.Nil(err)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?username=%s", tc.serverURL, username))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Equal(username, body.UserName)
}

func TestGetUserAuthenticated_ShouldIncludeAddress(t *testing.T) {
	assert := assert.New(t)

	userID := tc.user1.id
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/users/get?user_id=%s", tc.serverURL, userID), nil)
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Equal(userID, body.ID)
	assert.NotEmpty(body.Addresses)
}

func TestGetUserUnAuthenticated_ShouldNotIncludeAddress(t *testing.T) {
	assert := assert.New(t)

	userID := tc.user1.id
	resp, err := http.Get(fmt.Sprintf("%s/users/get?user_id=%s", tc.serverURL, userID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Equal(userID, body.ID)
	assert.Empty(body.Addresses)
}

// TODO: test user creation
// TODO: test creating user with DCInvestor then dCinvestor fails

func TestUpdateUserAuthenticated_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with user
	username := "BingBongBingBong"
	userID, err := persist.UserCreate(context.Background(), &persist.User{
		UserName:           username,
		UserNameIdempotent: strings.ToLower(username),
		Addresses:          []string{tc.user1.address},
		Bio:                "punk",
	}, tc.r)
	assert.Nil(err)
	jwt, err := jwtGeneratePipeline(context.Background(), userID, tc.r)
	assert.Nil(err)

	update := userUpdateInput{
		UserID:      userID,
		UserNameStr: "kaito",
	}
	data, err := json.Marshal(update)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update", tc.serverURL), bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	user, err := persist.UserGetByID(context.Background(), userID, tc.r)
	assert.Nil(err)
	assert.Equal(update.UserNameStr, user.UserName)
}

// Updating the username to itself should not trigger an error, despite the DB
// having a user entity with that username already
func TestUpdateUserAuthenticated_NoChange_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with user
	username := "BingBongBingBong"
	userID, err := persist.UserCreate(context.Background(), &persist.User{
		UserName:           username,
		UserNameIdempotent: strings.ToLower(username),
		Addresses:          []string{tc.user1.address},
		Bio:                "punk",
	}, tc.r)
	assert.Nil(err)
	jwt, err := jwtGeneratePipeline(context.Background(), userID, tc.r)
	assert.Nil(err)

	update := userUpdateInput{
		UserID:      userID,
		UserNameStr: username,
	}
	data, err := json.Marshal(update)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update", tc.serverURL), bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	user, err := persist.UserGetByID(context.Background(), userID, tc.r)
	assert.Nil(err)
	assert.Equal(update.UserNameStr, user.UserName)
}

func TestUpdateUserUnauthenticated_Failure(t *testing.T) {
	assert := assert.New(t)

	// seed DB with user
	username := "BingBongBingBing"
	userID, err := persist.UserCreate(context.Background(), &persist.User{
		UserName:           username,
		UserNameIdempotent: strings.ToLower(username),
		Addresses:          []string{tc.user1.address},
		Bio:                "punk",
	}, tc.r)
	assert.Nil(err)

	update := userUpdateInput{
		UserID:      userID,
		UserNameStr: "kaito",
	}
	data, err := json.Marshal(update)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update", tc.serverURL), bytes.NewBuffer(data))
	assert.Nil(err)
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	spew.Dump(resp.Body)
	assertValidJSONResponse(assert, resp)
}

func TestUpdateUserAuthenticated_UsernameTaken_Failure(t *testing.T) {
	assert := assert.New(t)

	// seed DB with user
	username := "BingBongBingBong"
	userID, err := persist.UserCreate(context.Background(), &persist.User{
		UserName:           username,
		UserNameIdempotent: strings.ToLower(username),
		Addresses:          []string{tc.user1.address},
		Bio:                "punk",
	}, tc.r)
	assert.Nil(err)
	jwt, err := jwtGeneratePipeline(context.Background(), userID, tc.r)
	assert.Nil(err)

	update := userUpdateInput{
		UserID:      userID,
		UserNameStr: tc.user1.username,
	}
	data, err := json.Marshal(update)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update", tc.serverURL), bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	user, err := persist.UserGetByID(context.Background(), userID, tc.r)
	assert.Nil(err)
	assert.NotEqual(update.UserNameStr, user.UserName)
}
