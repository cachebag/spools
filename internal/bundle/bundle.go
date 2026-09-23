package bundle

import "time"

const Version = 1

type Bundle struct {
	Version   int       `json:"version"`
	Tool      string    `json:"tool"`      // "zed", "cursor", ...
	SessionID string    `json:"sessionId"` // tool-native id
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`

	GitRemote string `json:"gitRemote,omitempty"`
	GitBranch string `json:"gitBranch,omitempty"`

	Origin Origin `json:"origin"`

	Payload []byte `json:"payload"`
}

type Origin struct {
	Machine     string `json:"machine"`
	ProjectRoot string `json:"projectRoot"`
	Home        string `json:"home"`
}
