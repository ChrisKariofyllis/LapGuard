package webui

import "io/fs"

// FS returns the embedded dashboard when the binary was built with -tags embedui
// after `npm run build`. Development and unit-test builds have no embed.
func FS() (fs.FS, bool) {
	return embeddedFS()
}
