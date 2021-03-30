package main

import (
	"net/http"
	
	"github.com/mikeydub/go-gallery.git/utils"
)

type galleryController struct {
	appConfig  applicationConfig
	httpClient http.Client
}

// might need this in the future
type authGrantResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int16  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

func newGalleryController(
	appConfig applicationConfig,
	httpClient http.Client,
) *galleryController {
	return &galleryController{
		appConfig,
		httpClient,
	}
}

func (g galleryController) handleLogin(w http.ResponseWriter, r *http.Request) {
	nonce := utils.RandomString(50)
	utils.AddCookie(w, "TEST_COOKIE_KEY", nonce)

	http.Redirect(w, r, "http://some_auth_url", http.StatusFound)
}

