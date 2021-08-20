package server

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/mikeydub/go-gallery/runtime"
)

type TestConfig struct {
	server          *httptest.Server
	serverURL       string
	r               *runtime.Runtime
	user1           *TestUser
	user2           *TestUser
}

var tc *TestConfig

func TestMain(m *testing.M) {
	tc = setup()
	code := m.Run()
	teardown(tc.server)
	os.Exit(code)
}
