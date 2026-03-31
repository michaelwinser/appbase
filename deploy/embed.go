// Package deploy provides embedded deployment assets.
package deploy

import (
	"embed"
	"io/fs"
)

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

// skillsFS holds the embedded consumer-facing skills.
//
//go:embed skills/*.md
var skillsFS embed.FS

// ConsumerSkills returns a map of filename → content for skills that
// should be distributed to consumer projects via `appbase update`.
func ConsumerSkills() map[string]string {
	skills := make(map[string]string)
	entries, err := fs.ReadDir(skillsFS, "skills")
	if err != nil {
		return skills
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(skillsFS, "skills/"+e.Name())
		if err != nil {
			continue
		}
		skills[e.Name()] = string(data)
	}
	return skills
}
