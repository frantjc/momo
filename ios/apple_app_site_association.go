package ios

type AppleAppSiteAssociation struct {
	AppLinks       AppLinks       `json:"applinks,omitempty"`
	WebCredentials WebCredentials `json:"webcredentials,omitempty"`
	AppClips       AppClips       `json:"appclips,omitempty"`
}

type AppLinks struct {
	Details []Details `json:"details,omitempty"`
}

type Details struct {
	AppIDs     []string    `json:"appIDs,omitempty"`
	Components []Component `json:"components,omitempty"`
}

type Component struct {
	Fragment string            `json:"#,omitempty"`
	Path     string            `json:"/,omitempty"`
	Query    map[string]string `json:"?,omitempty"`
	Exclude  bool              `json:"exclude,omitempty"`
	Comment  string            `json:"comment,omitempty"`
}

type WebCredentials struct {
	Apps []string `json:"apps,omitempty"`
}

type AppClips struct {
	Apps []string `json:"apps,omitempty"`
}
