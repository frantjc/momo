package apktool

type UsesFramework struct {
	IDs []int `yaml:"ids"`
	Tag any   `yaml:"tag"`
}

type SDKInfo struct {
	MinSDKVersion    int `yaml:"minSdkVersion"`
	TargetSDKVersion int `yaml:"targetSdkVersion"`
}

type PackageInfo struct {
	ForcedPackageID       int `yaml:"forcedPackageId"`
	RenameManifestPackage any `yaml:"renameManifestPackage"`
}

type VersionInfo struct {
	VersionCode int    `yaml:"versionCode"`
	VersionName string `yaml:"versionName"`
}

type Metadata struct {
	Version                string         `yaml:"version,omitempty"`
	APKFileName            string         `yaml:"apkFileName,omitempty"`
	IsFrameworkAPK         bool           `yaml:"isFrameworkApk,omitempty"`
	UsesFramework          *UsesFramework `yaml:"usesFramework,omitempty"`
	SDKInfo                *SDKInfo       `yaml:"sdkInfo,omitempty"`
	PackageInfo            *PackageInfo   `yaml:"packageInfo,omitempty"`
	VersionInfo            *VersionInfo   `yaml:"versionInfo,omitempty"`
	ResourcesAreCompressed bool           `yaml:"resourcesAreCompressed,omitempty"`
	SharedLibrary          bool           `yaml:"sharedLibrary,omitempty"`
	SparseResources        bool           `yaml:"sparseResources,omitempty"`
	UnknownFiles           map[string]int `yaml:"unknownFiles,omitempty"`
	DoNotCompress          []string       `yaml:"doNotCompress,omitempty"`
}
