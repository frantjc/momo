package momoregexp

import "regexp"

var (
	AppName    = regexp.MustCompile("^[a-zA-Z0-9-_]{1,32}$")
	AppVersion = regexp.MustCompile(`^[a-zA-Z0-9-_\.]{1,32}$`)
	UUID       = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")

	IPA = regexp.MustCompile(`(?i)^[\w/.-]+\.ipa$`)
	APK = regexp.MustCompile(`(?i)^[\w/.-]+\.apk$`)
	App = regexp.MustCompile(`(?i)^[\w/.-]+\.(ipa|apk)$`)
)
