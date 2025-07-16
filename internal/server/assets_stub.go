//go:build !assets

package server

import "embed"

var assetsFS embed.FS

const serveAssets = false
