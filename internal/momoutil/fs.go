package momoutil

import (
	// Embed the bytes of an image.
	_ "embed"
)

var (
	//go:embed question_mark.png
	questionMarkPNG []byte
)
