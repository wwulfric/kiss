package kiss

// Manifest 描述一个 skill 的入口、版本和 runner 约束。
type Manifest struct {
	Name        string
	Version     string
	Description string
	Entry       string
	RunnerType  string
}

// InstallRecord 是写入已安装 skill 目录的本地安装记录。
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

// Version 是 kiss --version 输出的版本号，release 构建时通过 ldflags 注入。
var Version = "dev"
