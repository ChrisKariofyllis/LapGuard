//go:build embedui

package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func embeddedFS() (fs.FS, bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	if f, err := sub.Open("index.html"); err != nil {
		return nil, false
	} else {
		_ = f.Close()
	}
	return sub, true
}
