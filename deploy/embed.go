// Package deploy provides embedded deployment assets.
package deploy

import _ "embed"

// DevTemplate is the contents of dev-template.sh, embedded at build time.
// Used by the `appbase dev-template` command to print the template to stdout.
//
//go:embed dev-template.sh
var DevTemplate string

// SandboxTemplate is the contents of sandbox-template.sh, embedded at build time.
// Used by the `appbase sandbox-template` command to print the template to stdout.
//
//go:embed sandbox-template.sh
var SandboxTemplate string
