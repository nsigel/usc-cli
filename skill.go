// Package usc holds what the usc command and a Go program share: the agent
// skill and the paths of the files that `usc auth login` saves.
package usc

import _ "embed"

// SkillMD is the agent skill that `usc skill` prints.
//
//go:embed SKILL.md
var SkillMD string
