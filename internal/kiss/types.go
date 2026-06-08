package kiss

type Manifest struct {
	Name        string
	Version     string
	Description string
	Entry       string
	RunnerType  string
}

type InstallRecord struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Kind        string `json:"kind"`
	Ref         string `json:"ref"`
	Resolved    string `json:"resolved"`
	SHA256      string `json:"sha256"`
	InstalledAt string `json:"installed_at"`
	KissVersion string `json:"kiss_version"`
}

var Version = "dev"
