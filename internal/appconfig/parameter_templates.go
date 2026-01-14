// internal/appconfig/parameter_templates.go
package appconfig

import (
	"fmt"
	"strings"
)

// ProfileName identifies a parameter preset/profile.
type ProfileName string

const (
	ProfileGenericChat ProfileName = "generic"
	ProfileFactChecker ProfileName = "fact_checker"
	ProfileCreative    ProfileName = "creative"
	ProfileAccuracy    ProfileName = "accuracy"
)

// ParamsForProfile selects a parameter profile by name.
// Behavior:
//   - empty string => Generic Chat (default)
//   - unknown string => Generic Chat (default)
func ParamsForProfile(name string) LlamaParams {
	n := normalizeProfileName(name)

	switch ProfileName(n) {
	case ProfileAccuracy:
		return DefaultAccuracyParams()
	case ProfileFactChecker:
		return DefaultFactCheckerParams()
	case ProfileCreative:
		return DefaultCreativeParams()
	case ProfileGenericChat:
		fallthrough
	default:
		return DefaultGenericChatParams()
	}
}

// DefaultGenericChatParams is the default profile when none is set in YAML.
func DefaultGenericChatParams() LlamaParams {
	return LlamaParams{
		Temperature: ptrFloat(0.8),  // Slightly higher for better flow
		TopP:        ptrFloat(1.0),  // Disable TopP to let MinP do the work
		TopK:        ptrInt(0),      // Disable TopK
		MinP:        ptrFloat(0.08), // Dynamic cutoff: very effective for 8B models
		TypicalP:    ptrFloat(1.0),

		RepeatLastN:      ptrInt(64),
		RepeatPenalty:    ptrFloat(1.1), // Lowered: high penalties cause "thesaurus syndrome"
		PresencePenalty:  ptrFloat(0.0), // Keep neutral for standard chat
		FrequencyPenalty: ptrFloat(0.0),

		Seed:     ptrInt64(-1),
		NPredict: ptrInt(1024),
		//Stream:   ptrBool(false),
	}
}

// DefaultAccuracyParams is tuned for absolute precision, zero-shot logic,
// and strict adherence to formatting constraints.
func DefaultAccuracyParams() LlamaParams {
	return LlamaParams{
		Temperature: ptrFloat(0.1),
		TopP:        ptrFloat(0.95), // Slightly wider for R1 stability
		MinP:        ptrFloat(0.1),

		// 512 is safe. Standard models will only use ~5 tokens.
		// R1 models will use ~200 for thinking and then stop.
		NPredict: ptrInt(512),

		RepeatPenalty: ptrFloat(1.0),
		Seed:          ptrInt64(42),
		//Stream:   ptrBool(false),
	}
}

// DefaultFactCheckerParams is tuned for short/precise answers, classification,
// math/facts, and accuracy-style benchmarking.
func DefaultFactCheckerParams() LlamaParams {
	return LlamaParams{
		Temperature: ptrFloat(0.2),
		TopP:        ptrFloat(0.6),
		TopK:        ptrInt(20),
		MinP:        ptrFloat(0.1),
		TypicalP:    ptrFloat(0.8),

		RepeatLastN:      ptrInt(128),
		RepeatPenalty:    ptrFloat(1.05),
		PresencePenalty:  ptrFloat(0.0),
		FrequencyPenalty: ptrFloat(0.0),

		Seed:     ptrInt64(42), // deterministic (change/remove if you want randomness)
		NPredict: ptrInt(64),   // short outputs by default
		//Stream:   ptrBool(false),
	}
}

// DefaultCreativeParams is tuned for creative writing, brainstorming, and
// stylistic variance (at the cost of determinism).
func DefaultCreativeParams() LlamaParams {
	return LlamaParams{
		Temperature: ptrFloat(1.5), // High heat for "sparkle"...
		TopP:        ptrFloat(1.0),
		TopK:        ptrInt(0),
		MinP:        ptrFloat(0.15), // ...but a strict filter to prune "garbage" tokens
		TypicalP:    ptrFloat(0.9),  // Keeps the output feeling "natural"

		RepeatLastN:      ptrInt(256),
		RepeatPenalty:    ptrFloat(1.05),
		PresencePenalty:  ptrFloat(0.5), // Encourages introducing new topics/words
		FrequencyPenalty: ptrFloat(0.2), // Discourages repeating the same phrases

		Seed:     ptrInt64(-1),
		NPredict: ptrInt(2048),
		//Stream:   ptrBool(false),
	}
}

func applyParameterTemplates(config *Config) error {
	for i := range config.Hosts {
		host := &config.Hosts[i]
		if strings.TrimSpace(host.ParameterTemplate) == "" {
			name := strings.TrimSpace(host.Name)
			if name == "" {
				name = host.URL
			}
			if name == "" {
				name = "unknown-host"
			}
			return fmt.Errorf("parameterTemplate is required for host %q", name)
		}
		template := ParamsForProfile(host.ParameterTemplate)
		host.Parameters = mergeParams(template, host.Parameters)
	}
	return nil
}

// ApplyParameterTemplates applies parameter profiles to every host.
func ApplyParameterTemplates(config *Config) error {
	return applyParameterTemplates(config)
}

func mergeParams(base LlamaParams, override LlamaParams) LlamaParams {
	if override.Temperature != nil {
		base.Temperature = override.Temperature
	}
	if override.TopK != nil {
		base.TopK = override.TopK
	}
	if override.TopP != nil {
		base.TopP = override.TopP
	}
	if override.MinP != nil {
		base.MinP = override.MinP
	}
	if override.TypicalP != nil {
		base.TypicalP = override.TypicalP
	}
	if override.DynaTempRange != nil {
		base.DynaTempRange = override.DynaTempRange
	}
	if override.DynaTempExponent != nil {
		base.DynaTempExponent = override.DynaTempExponent
	}
	if override.Mirostat != nil {
		base.Mirostat = override.Mirostat
	}
	if override.MirostatTau != nil {
		base.MirostatTau = override.MirostatTau
	}
	if override.MirostatEta != nil {
		base.MirostatEta = override.MirostatEta
	}
	if override.XTCProbability != nil {
		base.XTCProbability = override.XTCProbability
	}
	if override.XTCThreshold != nil {
		base.XTCThreshold = override.XTCThreshold
	}
	if override.Samplers != nil {
		base.Samplers = override.Samplers
	}
	if override.RepeatLastN != nil {
		base.RepeatLastN = override.RepeatLastN
	}
	if override.RepeatPenalty != nil {
		base.RepeatPenalty = override.RepeatPenalty
	}
	if override.PresencePenalty != nil {
		base.PresencePenalty = override.PresencePenalty
	}
	if override.FrequencyPenalty != nil {
		base.FrequencyPenalty = override.FrequencyPenalty
	}
	if override.DryMultiplier != nil {
		base.DryMultiplier = override.DryMultiplier
	}
	if override.DryBase != nil {
		base.DryBase = override.DryBase
	}
	if override.DryAllowedLength != nil {
		base.DryAllowedLength = override.DryAllowedLength
	}
	if override.DryPenaltyLastN != nil {
		base.DryPenaltyLastN = override.DryPenaltyLastN
	}
	if override.DrySequenceBreakers != nil {
		base.DrySequenceBreakers = override.DrySequenceBreakers
	}
	if override.NPredict != nil {
		base.NPredict = override.NPredict
	}
	if override.Stop != nil {
		base.Stop = override.Stop
	}
	if override.IgnoreEOS != nil {
		base.IgnoreEOS = override.IgnoreEOS
	}
	if override.TMaxPredictMS != nil {
		base.TMaxPredictMS = override.TMaxPredictMS
	}
	if override.Seed != nil {
		base.Seed = override.Seed
	}
	if override.LogitBias != nil {
		base.LogitBias = override.LogitBias
	}
	if override.NProbs != nil {
		base.NProbs = override.NProbs
	}
	if override.PostSamplingProbs != nil {
		base.PostSamplingProbs = override.PostSamplingProbs
	}
	if override.ReturnTokens != nil {
		base.ReturnTokens = override.ReturnTokens
	}
	if override.MinKeep != nil {
		base.MinKeep = override.MinKeep
	}
	if override.NKeep != nil {
		base.NKeep = override.NKeep
	}
	if override.CachePrompt != nil {
		base.CachePrompt = override.CachePrompt
	}
	if override.NCacheReuse != nil {
		base.NCacheReuse = override.NCacheReuse
	}
	if override.Stream != nil {
		base.Stream = override.Stream
	}
	if override.TimingsPerToken != nil {
		base.TimingsPerToken = override.TimingsPerToken
	}
	if override.ReturnProgress != nil {
		base.ReturnProgress = override.ReturnProgress
	}
	if override.ResponseFields != nil {
		base.ResponseFields = override.ResponseFields
	}
	if override.NIndent != nil {
		base.NIndent = override.NIndent
	}
	if override.IDSlot != nil {
		base.IDSlot = override.IDSlot
	}
	if override.Grammar != nil {
		base.Grammar = override.Grammar
	}
	if override.JSONSchema != nil {
		base.JSONSchema = override.JSONSchema
	}
	if override.NCmpl != nil {
		base.NCmpl = override.NCmpl
	}
	if override.Lora != nil {
		base.Lora = override.Lora
	}

	return base
}

func normalizeProfileName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	// allow a few friendly aliases
	switch s {
	case "", "default", "chat", "generic_chat", "generic-chat":
		return string(ProfileGenericChat)
	case "accuracy", "acc":
		return string(ProfileAccuracy)
	case "fact", "factchecker", "fact-checker", "fact_check", "fact-check":
		return string(ProfileFactChecker)
	case "creative_writing", "creative-writing", "writer":
		return string(ProfileCreative)
	default:
		return s
	}
}

// Pointer helpers (keeps structs clean + preserves unset vs explicitly set).
func ptrInt(v int) *int           { return &v }
func ptrInt64(v int64) *int64     { return &v }
func ptrFloat(v float64) *float64 { return &v }
func ptrBool(v bool) *bool        { return &v }
