package runtime

import (
	"path/filepath"
	"runtime"
	"os"
	"fmt"
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
	envPath := ".env"
	envIsLocal := fileExists(envPath)
	if !envIsLocal {
		envPath = fmt.Sprintf("%s/bin/.env", ProjectRootPath)
	}
	return envPath
}
