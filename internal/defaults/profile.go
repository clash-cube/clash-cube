// Package defaults contains the starter configuration shared by app frontends.
package defaults

import _ "embed"

//go:embed profile.yaml
var Profile []byte
