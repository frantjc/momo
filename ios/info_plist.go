package ios

import (
	_ "howett.net/plist"
)

type Info struct {
	BuildMachineOSBuild       string   `plist:"BuildMachineOSBuild"`
	CFBundleDevelopmentRegion string   `plist:"CFBundleDevelopmentRegion"`
	CFBundleDisplayName       string   `plist:"CFBundleDisplayName"`
	CFBundleExecutable        string   `plist:"CFBundleExecutable"`
	CFBundleIconFiles         []string `plist:"CFBundleIconFiles"`
	CFBundleIcons             *struct {
		CFBundlePrimaryIcon *struct {
			CFBundleIconFiles []string `plist:"CFBundleIconFiles"`
		} `plist:"CFBundlePrimaryIcon"`
	} `plist:"CFBundleIcons"`
	CFBundleIdentifier            string   `plist:"CFBundleIdentifier"`
	CFBundleInfoDictionaryVersion string   `plist:"CFBundleInfoDictionaryVersion"`
	CFBundleName                  string   `plist:"CFBundleName"`
	CFBundlePackageType           string   `plist:"CFBundlePackageType"`
	CFBundleShortVersionString    string   `plist:"CFBundleShortVersionString"`
	CFBundleSupportedPlatforms    []string `plist:"CFBundleSupportedPlatforms"`
	CFBundleVersion               string   `plist:"CFBundleVersion"`
}
