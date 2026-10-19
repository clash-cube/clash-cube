package backend

// HelperStatus is what Settings shows of service mode.
type HelperStatus struct {
	Installed bool `json:"installed"`
	Current   bool `json:"current"` // runs this version of the app
	Enabled   bool `json:"enabled"` // settings.ServiceMode
	Active    bool `json:"active"`  // the running core is the helper's
}
