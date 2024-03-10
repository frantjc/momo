package momoregexp

func IsAppName(name string) bool {
	return AppName.MatchString(name)
}

func IsAppVersion(name string) bool {
	return AppVersion.MatchString(name)
}

func IsUUID(name string) bool {
	return UUID.MatchString(name)
}

func IsIPA(name string) bool {
	return IPA.MatchString(name)
}

func IsAPK(name string) bool {
	return APK.MatchString(name)
}

func IsApp(name string) bool {
	return App.MatchString(name)
}
