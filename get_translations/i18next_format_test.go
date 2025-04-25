package main

import "testing"

func TestParseFormatKey(t *testing.T) {
	result := parseFormatKey("import.drop-here %(filename)s")
	if result != "filename" {
		t.Error("Unexpected result:", result)
	}
}

func TestPythonToI18Next(t *testing.T) {
	tests := []struct {
		name        string
		translation string
		formatKey   string
		expected    string
	}{
		{
			name:        "Basic replacement",
			translation: "Hello %(name)s!",
			formatKey:   "name",
			expected:    "Hello {{name}}!",
		},
		{
			name:        "Multiple occurrences",
			translation: "%(user)s logged in. Welcome %(user)s!",
			formatKey:   "user",
			expected:    "{{user}} logged in. Welcome {{user}}!",
		},
		{
			name:        "No match",
			translation: "Hello world!",
			formatKey:   "name",
			expected:    "Hello world!",
		},
		{
			name:        "Empty translation",
			translation: "",
			formatKey:   "key",
			expected:    "",
		},
		{
			name:        "Empty format key",
			translation: "Test %(key)s value",
			formatKey:   "",
			expected:    "Test %(key)s value",
		},
		{
			name:        "Different format keys",
			translation: "Hello %(name)s, your ID is %(id)s",
			formatKey:   "name",
			expected:    "Hello {{name}}, your ID is %(id)s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pythonToI18Next(tt.translation, tt.formatKey)
			if result != tt.expected {
				t.Errorf("pythonToI18Next(%q, %q) = %q, want %q",
					tt.translation, tt.formatKey, result, tt.expected)
			}
		})
	}
}
