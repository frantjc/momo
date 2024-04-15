package android_test

import (
	"bytes"
	"encoding/xml"
	"testing"

	// Used to embed the bytes of test XML.
	_ "embed"

	"github.com/frantjc/momo/android"
)

var (
	//go:embed AndroidManifest.test.xml
	data []byte
)

func TestUnmarshalAndroindManifest(t *testing.T) {
	manifest := &android.Manifest{}
	if err := xml.NewDecoder(bytes.NewReader(data)).Decode(manifest); err != nil {
		t.Error(err)
		t.FailNow()
	}
}
