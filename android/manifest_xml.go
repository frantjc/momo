package android

import "encoding/xml"

const (
	AndroidManifestName = "AndroidManifest.xml"
)

type Manifest struct {
	XMLName        xml.Name                 `xml:"manifest"`
	UsesPermission []ManifestUsesPermission `xml:"uses-permission"`
	UsesFeature    []ManifestUsesFeature    `xml:"uses-feature"`
	Permission     []ManifestPermission     `xml:"permission"`
	Application    ManifestApplication      `xml:"application"`
	Attrs          []xml.Attr               `xml:",any,attr"`
}

func (m *Manifest) Package() string {
	for _, attr := range m.Attrs {
		if attr.Name.Local == "package" {
			return attr.Value
		}
	}

	return ""
}

type ManifestUsesPermission struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

type ManifestUsesFeature struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

type ManifestPermission struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

type ManifestApplication struct {
	Activities      []ManifestApplicationActivity `xml:"activity"`
	ActivityAliases []ManifestApplicationActivity `xml:"activity-alias"`
	Receivers       []ManifestApplicationActivity `xml:"receiver"`
	Services        []ManifestApplicationActivity `xml:"service"`
	Providers       []ManifestApplicationActivity `xml:"providers"`
	UsesLibraries   []ManifestApplicationMetadata `xml:"uses-library"`
	Attrs           []xml.Attr                    `xml:",any,attr"`
}

type ManifestApplicationActivity struct {
	Metadata     ManifestApplicationMetadata     `xml:"metadata"`
	IntentFilter ManifestApplicationIntentFilter `xml:"intent-filter"`
	Attrs        []xml.Attr                      `xml:",any,attr"`
}

type ManifestApplicationIntentFilter struct {
	Actions    []ManifestApplicationMetadata `xml:"action"`
	Categories []ManifestApplicationMetadata `xml:"category"`
}

type ManifestApplicationMetadata struct {
	Attrs []xml.Attr `xml:",any,attr"`
}
