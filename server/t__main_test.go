package server

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/mikeydub/go-gallery/runtime"
)

var testServer *httptest.Server
var serverUrl string
var r *runtime.Runtime

func TestMain(m *testing.M) {
    testServer, serverUrl, r = setup()
    code := m.Run() 
    teardown(testServer)
    os.Exit(code)
}