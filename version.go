package flow

import _ "embed"

//go:embed VERSION
var version string

// Version will return the flow release version
func Version() string {
	return version
}
