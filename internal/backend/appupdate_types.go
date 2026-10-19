package backend

// AppUpdate is where updating the app stands, for Settings and the tray.
type AppUpdate struct {
	// idle | checking | latest | downloading | ready | installed | available | source | error
	State   string `json:"state"`
	Latest  string `json:"latest,omitempty"`
	Notes   string `json:"notes,omitempty"`
	URL     string `json:"url,omitempty"`   // the release page
	Stuck   string `json:"stuck,omitempty"` // available: why it can't replace itself
	Error   string `json:"error,omitempty"`
	Done    int64  `json:"done,omitempty"` // downloading: bytes so far, of total (0 when unknown)
	Total   int64  `json:"total,omitempty"`
	Checked int64  `json:"checked,omitempty"` // unix ms of the last answer
}
