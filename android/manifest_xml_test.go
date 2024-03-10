package android

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"testing"
)

var (
	//go:embed AndroidManifest.test.xml
	data []byte
)

func TestUnmarshalAndroindManifest(t *testing.T) {
	manifest := &Manifest{}
	if err := xml.NewDecoder(bytes.NewReader(data)).Decode(manifest); err != nil {
		t.Error(err)
		t.FailNow()
	}
}
