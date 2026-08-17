//go:build !embedui

package webui

import "io/fs"

func embeddedFS() (fs.FS, bool) {
	return nil, false
}
