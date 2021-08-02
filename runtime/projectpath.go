package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var (
	_, b, _, _ = runtime.Caller(0)

	// Root folder of this project
	ProjectRootPath = filepath.Join(filepath.Dir(b), "..")
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func getEnvPath() string {
	localEnvPath := ".env"
	if fileExists(localEnvPath) {
		return localEnvPath
	}

	binEnvPath := fmt.Sprintf("%s/bin/.env", ProjectRootPath)
	if fileExists(binEnvPath) {
		return binEnvPath
	}

	return ""
}
