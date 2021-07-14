package glry_core

import (
	"path/filepath"
	"runtime"
)

var (
    _, b, _, _ = runtime.Caller(0)

    // Root folder of this project
    ProjectRootPath = filepath.Join(filepath.Dir(b), "..")
)