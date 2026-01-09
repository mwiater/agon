// internal/models/parameters.go
package models

import "github.com/mwiater/agon/internal/appconfig"

// ProfileName identifies a parameter preset/profile.
type ProfileName = appconfig.ProfileName

const (
	ProfileGenericChat = appconfig.ProfileGenericChat
	ProfileFactChecker = appconfig.ProfileFactChecker
	ProfileCreative    = appconfig.ProfileCreative
)

// ParamsForProfile selects a parameter profile by name.
// Behavior:
//   - empty string => Generic Chat (default)
//   - unknown string => Generic Chat (default)
func ParamsForProfile(name string) appconfig.LlamaParams {
	return appconfig.ParamsForProfile(name)
}

// DefaultGenericChatParams is the default profile when none is set in YAML.
func DefaultGenericChatParams() appconfig.LlamaParams {
	return appconfig.DefaultGenericChatParams()
}

// DefaultFactCheckerParams is tuned for short/precise answers, classification,
// math/facts, and accuracy-style benchmarking.
func DefaultFactCheckerParams() appconfig.LlamaParams {
	return appconfig.DefaultFactCheckerParams()
}

// DefaultCreativeParams is tuned for creative writing, brainstorming, and
// stylistic variance (at the cost of determinism).
func DefaultCreativeParams() appconfig.LlamaParams {
	return appconfig.DefaultCreativeParams()
}
