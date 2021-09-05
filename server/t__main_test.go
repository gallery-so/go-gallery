package server

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/mikeydub/go-gallery/runtime"
)

type TestConfig struct {
	server    *httptest.Server
	serverURL string
	r         *runtime.Runtime
	user1     *TestUser
	user2     *TestUser
}

var tc *TestConfig

func TestMain(m *testing.M) {
	tc = setup()
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(r)
			teardown()
		} else {
			teardown()
		}
	}()
	m.Run()
}
