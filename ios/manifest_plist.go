package ios

import (
	// Document what package can be used to unmarshal
	// a manifest.plist into the struct below.
	_ "howett.net/plist"
)

type Manifest struct {
	Items []ManifestItem `plist:"items"`
}

type ManifestItem struct {
	Assets   []ManifestItemAsset   `plist:"assets"`
	Metadata *ManifestItemMetadata `plist:"metadata"`
}

type ManifestItemAsset struct {
	Kind string `plist:"kind"`
	URL  string `plist:"url"`
}

type ManifestItemMetadata struct {
	BundleIdentifier   string `plist:"bundle-identifier"`
	BundleVersion      string `plist:"bundle-version"`
	Kind               string `plist:"kind"`
	PlatformIdentifier string `plist:"platform-identifier"`
	Title              string `plist:"title"`
}
