package autocorrect

import "testing"

func TestCorrectWord_OverCapitalization(t *testing.T) {
	c := New(true)
	tests := []struct {
		input string
		want  string
	}{
		{"THe", "The"},
		{"HEllo", "Hello"},
		{"WOrld", "World"},
		{"ABCdef", "Abcdef"},
		{"FOr", "For"},
	}
	for _, tt := range tests {
		got := c.CorrectWord(tt.input)
		if got == nil {
			t.Errorf("CorrectWord(%q) = nil, want correction to %q", tt.input, tt.want)
			continue
		}
		if got.Corrected != tt.want {
			t.Errorf("CorrectWord(%q).Corrected = %q, want %q", tt.input, got.Corrected, tt.want)
		}
		if got.Original != tt.input {
			t.Errorf("CorrectWord(%q).Original = %q, want %q", tt.input, got.Original, tt.input)
		}
	}
}

func TestCorrectWord_AllCapsPreserved(t *testing.T) {
	c := New(true)
	// ALL-CAPS words should not be corrected (likely acronyms)
	for _, word := range []string{"TODO", "API", "HTTP", "SQL", "CLI", "OK", "UI"} {
		if got := c.CorrectWord(word); got != nil {
			t.Errorf("CorrectWord(%q) = %+v, want nil (all-caps should be preserved)", word, got)
		}
	}
}

func TestCorrectWord_TypoDictionary(t *testing.T) {
	c := New(true)
	tests := []struct {
		input string
		want  string
	}{
		{"teh", "the"},
		{"adn", "and"},
		{"jsut", "just"},
		{"recieve", "receive"},
		{"seperate", "separate"},
		{"definately", "definitely"},
	}
	for _, tt := range tests {
		got := c.CorrectWord(tt.input)
		if got == nil {
			t.Errorf("CorrectWord(%q) = nil, want correction to %q", tt.input, tt.want)
			continue
		}
		if got.Corrected != tt.want {
			t.Errorf("CorrectWord(%q).Corrected = %q, want %q", tt.input, got.Corrected, tt.want)
		}
	}
}

func TestCorrectWord_PreservesCase(t *testing.T) {
	c := New(true)

	// Capitalized input → capitalized correction
	got := c.CorrectWord("Teh")
	if got == nil || got.Corrected != "The" {
		t.Errorf("CorrectWord(\"Teh\") = %+v, want correction to \"The\"", got)
	}

	// All-caps typo → all-caps correction
	got = c.CorrectWord("TEH")
	if got == nil || got.Corrected != "THE" {
		t.Errorf("CorrectWord(\"TEH\") = %+v, want correction to \"THE\"", got)
	}

	// Lowercase stays lowercase
	got = c.CorrectWord("teh")
	if got == nil || got.Corrected != "the" {
		t.Errorf("CorrectWord(\"teh\") = %+v, want correction to \"the\"", got)
	}
}

func TestCorrectWord_NoChangeNeeded(t *testing.T) {
	c := New(true)
	for _, word := range []string{"the", "hello", "world", "The", "Hello", "correct", "meeting"} {
		if got := c.CorrectWord(word); got != nil {
			t.Errorf("CorrectWord(%q) = %+v, want nil (no change needed)", word, got)
		}
	}
}

func TestCorrectWord_ShortAndEmpty(t *testing.T) {
	c := New(true)
	for _, word := range []string{"", "I", "a", "x"} {
		if got := c.CorrectWord(word); got != nil {
			t.Errorf("CorrectWord(%q) = %+v, want nil", word, got)
		}
	}
}

func TestCorrectWord_Disabled(t *testing.T) {
	c := New(false)
	// Even known typos should return nil when disabled
	for _, word := range []string{"teh", "THe", "adn"} {
		if got := c.CorrectWord(word); got != nil {
			t.Errorf("disabled: CorrectWord(%q) = %+v, want nil", word, got)
		}
	}
}

func TestFixOverCap_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Ab", "Ab"},       // only 1 leading uppercase, no fix
		{"AB", "AB"},       // all caps, preserve
		{"ABc", "Abc"},     // 2 upper + 1 lower, fix
		{"HELLO", "HELLO"}, // all caps, preserve
		{"hello", "hello"}, // all lower, no fix
		{"iPhone", "iPhone"}, // camelCase-like, no fix (uppercase not leading)
	}
	for _, tt := range tests {
		got := fixOverCap(tt.input)
		if got != tt.want {
			t.Errorf("fixOverCap(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
