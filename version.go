package search

import (
	"bytes"
	"encoding/json"
	"time"
)

// VersionFile is the current.json pointer for a shard or a container.
// An empty Encoding means the object is raw bytes. "gzip" is a label the
// caller sets; this struct does not compress.
type VersionFile struct {
	ObjectKey string      `json:"object_key"`
	ETag      string      `json:"etag"`
	Size      int64       `json:"size"`
	BuiltAt   time.Time   `json:"built_at,omitempty"`
	DocCount  *uint32     `json:"doc_count,omitempty"`
	IsEnabled *bool       `json:"is_enabled,omitempty"`
	IsHidden  *bool       `json:"is_hidden,omitempty"`
	PublicOK  *bool       `json:"public_ok,omitempty"`
	Encoding  string      `json:"encoding,omitempty"`
	Keep      []KeepEntry `json:"keep,omitempty"`
}

// KeepEntry is one version bin the tidy pass must spare until Until.
type KeepEntry struct {
	ObjectKey string    `json:"object_key"`
	Until     time.Time `json:"until"`
}

// Marshal encodes the pointer as JSON. An empty Encoding is omitted.
func (v VersionFile) Marshal() ([]byte, error) {
	return json.Marshal(v)
}

// ParseVersionFile decodes a current.json pointer. Unknown fields are rejected.
func ParseVersionFile(b []byte) (VersionFile, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var out VersionFile
	if err := dec.Decode(&out); err != nil {
		return VersionFile{}, err
	}
	return out, nil
}
