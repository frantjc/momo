package android

type AssetLink struct {
	Relation []string `json:"relation,omitempty"`
	Target   Target   `json:"target,omitempty"`
}

type Target struct {
	Namespace              string   `json:"namespace,omitempty"`
	PackageName            string   `json:"package_name,omitempty"`
	SHA256CertFingerprints []string `json:"sha256_cert_fingerprints,omitempty"`
}
