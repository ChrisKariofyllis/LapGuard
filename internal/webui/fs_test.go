//go:build !embedui

package webui

import "testing"

func TestFSUnavailableWithoutEmbedTag(t *testing.T) {
	if _, ok := FS(); ok {
		t.Fatal("unit tests must not require the embedded dashboard")
	}
}
