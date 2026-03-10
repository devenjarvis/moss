package autocorrect

import (
	"strings"
	"unicode"
)

// Correction records a single autocorrect replacement so it can be undone.
type Correction struct {
	Original  string // the word before correction
	Corrected string // the word after correction
}

// Corrector performs lightweight autocorrect: over-capitalization fixes and
// common typo replacements. It is intentionally conservative — only clearly
// wrong words are corrected, and corrections can be undone via backspace.
type Corrector struct {
	enabled bool
	typos   map[string]string
}

// New creates a Corrector. Pass enabled=false to create a no-op corrector.
func New(enabled bool) *Corrector {
	return &Corrector{
		enabled: enabled,
		typos:   defaultTypos,
	}
}

// CorrectWord checks a single word and returns a Correction if a fix applies.
// Returns nil if no correction is needed.
func (c *Corrector) CorrectWord(word string) *Correction {
	if !c.enabled || len(word) < 2 {
		return nil
	}

	// 1. Over-capitalization fix: "THe" → "The", "HEllo" → "Hello"
	if fixed := fixOverCap(word); fixed != word {
		return &Correction{Original: word, Corrected: fixed}
	}

	// 2. Typo dictionary lookup
	lower := strings.ToLower(word)
	replacement, ok := c.typos[lower]
	if !ok {
		return nil
	}

	// Preserve original casing pattern
	corrected := matchCase(word, replacement)
	if corrected == word {
		return nil
	}
	return &Correction{Original: word, Corrected: corrected}
}

// fixOverCap fixes words where shift was held too long, e.g. "THe" → "The".
// It detects 2+ leading uppercase letters followed by 1+ lowercase letters,
// skipping ALL-CAPS words (likely intentional acronyms like "TODO", "API").
func fixOverCap(word string) string {
	runes := []rune(word)
	if len(runes) < 2 {
		return word
	}

	// Count leading uppercase runes
	upperCount := 0
	for _, r := range runes {
		if unicode.IsUpper(r) {
			upperCount++
		} else {
			break
		}
	}

	// Need at least 2 leading uppercase and at least 1 following lowercase
	if upperCount < 2 || upperCount >= len(runes) {
		return word // all-caps or not enough uppercase
	}

	// Check that everything after the uppercase prefix is lowercase/non-letter
	hasLower := false
	for _, r := range runes[upperCount:] {
		if unicode.IsUpper(r) {
			return word // mixed case in the tail, don't touch (e.g. "McGraw")
		}
		if unicode.IsLower(r) {
			hasLower = true
		}
	}
	if !hasLower {
		return word
	}

	// Fix: uppercase first letter, lowercase the rest
	result := make([]rune, len(runes))
	result[0] = unicode.ToUpper(runes[0])
	for i := 1; i < len(runes); i++ {
		result[i] = unicode.ToLower(runes[i])
	}
	return string(result)
}

// matchCase applies the casing pattern of original to replacement.
func matchCase(original, replacement string) string {
	if len(original) == 0 {
		return replacement
	}
	origRunes := []rune(original)

	// All uppercase → return uppercase replacement
	allUpper := true
	for _, r := range origRunes {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			allUpper = false
			break
		}
	}
	if allUpper {
		return strings.ToUpper(replacement)
	}

	// First letter uppercase → capitalize replacement
	if unicode.IsUpper(origRunes[0]) {
		reps := []rune(replacement)
		reps[0] = unicode.ToUpper(reps[0])
		return string(reps)
	}

	// Default: lowercase
	return replacement
}

// defaultTypos maps common misspellings to their corrections. Only includes
// words that are never valid English words. Kept conservative to avoid
// frustrating corrections in tech/brand contexts.
var defaultTypos = map[string]string{
	// Transposed/fat-fingered letters
	"teh":     "the",
	"hte":     "the",
	"adn":     "and",
	"nad":     "and",
	"taht":    "that",
	"thta":    "that",
	"wiht":    "with",
	"whit":    "with",
	"waht":    "what",
	"hwat":    "what",
	"tihs":    "this",
	"htis":    "this",
	"jsut":    "just",
	"nto":     "not",
	"hsa":     "has",
	"ahve":    "have",
	"hvae":    "have",
	"cna":     "can",
	"liek":    "like",
	"lkie":    "like",
	"yuo":     "you",
	"freom":   "from",
	"fomr":    "from",
	"thn":     "than",
	"hten":    "then",
	"thne":    "then",
	"thier":   "their",
	"tehir":   "their",
	"woudl":   "would",
	"wuold":   "would",
	"cuold":   "could",
	"shoudl":  "should",
	"shuold":  "should",
	"knwo":    "know",
	"konw":    "know",
	"soem":    "some",
	"smoe":    "some",
	"whne":    "when",
	"wehn":    "when",
	"baout":   "about",
	"abotu":   "about",
	"beacuse": "because",
	"becuase": "because",
	"whcih":   "which",
	"wihch":   "which",

	// Common misspellings
	"recieve":     "receive",
	"beleive":     "believe",
	"occured":     "occurred",
	"occuring":    "occurring",
	"seperate":    "separate",
	"definately":  "definitely",
	"definitly":   "definitely",
	"occassion":   "occasion",
	"accomodate":  "accommodate",
	"neccessary":  "necessary",
	"necessery":   "necessary",
	"untill":      "until",
	"acheive":     "achieve",
	"arguement":   "argument",
	"enviroment":  "environment",
	"goverment":   "government",
	"independant": "independent",
	"managment":   "management",
	"realy":       "really",
	"similiar":    "similar",
	"tommorow":    "tomorrow",
	"togehter":    "together",
	"togather":    "together",
}
