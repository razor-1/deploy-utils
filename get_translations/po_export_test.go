package main

import "testing"

func TestLocaleFromPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Serbian Latin script with @ notation",
			input:    "/translations/sr@latn/LC_MESSAGES",
			expected: "sr-Latn",
		},
		{
			name:     "Serbian Cyrillic script with @ notation",
			input:    "/translations/sr@cyrl/LC_MESSAGES",
			expected: "sr-Cyrl",
		},
		{
			name:     "Simple locale without script",
			input:    "/translations/en-US/LC_MESSAGES",
			expected: "en-US",
		},
		{
			name:     "Simple locale without region",
			input:    "/translations/fr/LC_MESSAGES",
			expected: "fr",
		},
		{
			name:     "Path with only one component",
			input:    "/translations",
			expected: "",
		},
		{
			name:     "Empty path",
			input:    "",
			expected: "",
		},
		{
			name:     "Path with multiple @ symbols (edge case)",
			input:    "/translations/zh@hans@test/LC_MESSAGES",
			expected: "zh-Hans",
		},
		{
			name:     "Locale with underscore",
			input:    "/translations/pt_BR/LC_MESSAGES",
			expected: "pt_BR",
		},
		{
			name:     "Different base directory structure",
			input:    "/var/locale/data/es/LC_MESSAGES",
			expected: "es",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := localeFromPath(tt.input)
			if result != tt.expected {
				t.Errorf("localeFromPath(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
