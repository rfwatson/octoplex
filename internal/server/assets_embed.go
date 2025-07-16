//go:build assets

package server

import (
	"embed"
)

//go:embed assets/*
var assetsFS embed.FS

const serveAssets = true
