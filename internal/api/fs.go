package api

import (
	// Embed the bytes of some files.
	_ "embed"
)

var (
	//go:embed question_mark.png
	questionMarkPNG []byte
	//go:embed swagger.json
	swaggerJSON []byte
)
