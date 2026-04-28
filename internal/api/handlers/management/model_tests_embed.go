package management

import _ "embed"

//go:embed assets/model-tests.html
var modelTestsPageHTML []byte

// ModelTestsPageHTML returns the embedded single-file management UI for the
// model-tests dashboard. The server serves it as a public asset and the
// dashboard authenticates with the management key from localStorage.
func ModelTestsPageHTML() []byte {
	return modelTestsPageHTML
}
