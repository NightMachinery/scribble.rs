package wordpacks

import "embed"

// Files contains the built-in wordpacks shipped with the binary.
//
//go:embed *
var Files embed.FS
