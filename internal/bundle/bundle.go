package bundle

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

const Version = 1

type Bundle struct {
	Version   int               `json:"version"`
	Tool      string            `json:"tool"`
	SessionID string            `json:"sessionId"`
	Title     string            `json:"title"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Meta      map[string]string `json:"meta,omitempty"` // parent_id, folder_paths, folder_paths_order, created_at, data_type

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

func Write(w io.Writer, b *Bundle) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(b)
}

func Read(r io.Reader) (*Bundle, error) {
	var b Bundle
	if err := json.NewDecoder(r).Decode(&b); err != nil {
		return nil, fmt.Errorf("reading bundle: %w", err)
	}
	if b.Version != Version {
		return nil, fmt.Errorf("unsupported bundle version %d (want %d)", b.Version, Version)
	}
	if b.SessionID == "" {
		return nil, fmt.Errorf("bundle has no session id")
	}
	return &b, nil
}
