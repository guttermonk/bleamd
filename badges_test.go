package main

import (
	"strings"
	"testing"
)

func TestParseShieldsBadge(t *testing.T) {
	tests := []struct {
		name          string
		badgeURL      string
		expectedLabel string
		expectedMsg   string
	}{
		{
			name:          "GitHub license badge",
			badgeURL:      "https://img.shields.io/github/license/guttermonk/bleamd.svg?style=for-the-badge",
			expectedLabel: "license",
			expectedMsg:   "guttermonk/bleamd",
		},
		{
			name:          "GitHub stars badge",
			badgeURL:      "https://img.shields.io/github/stars/guttermonk/bleamd?style=for-the-badge",
			expectedLabel: "stars",
			expectedMsg:   "guttermonk/bleamd",
		},
		{
			name:          "Static badge",
			badgeURL:      "https://img.shields.io/badge/build-passing-green",
			expectedLabel: "build",
			expectedMsg:   "passing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, message, _ := parseShieldsBadge(tt.badgeURL)
			if label != tt.expectedLabel {
				t.Errorf("Expected label %q, got %q", tt.expectedLabel, label)
			}
			if message != tt.expectedMsg {
				t.Errorf("Expected message %q, got %q", tt.expectedMsg, message)
			}
		})
	}
}

func TestProcessBadges(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "Linked GitHub license badge",
			input:    "[![GitHub license](https://img.shields.io/github/license/guttermonk/bleamd.svg)](https://github.com/guttermonk/bleamd/blob/master/LICENSE)",
			contains: "[license: guttermonk/bleamd]",
		},
		{
			name:     "Linked GitHub stars badge",
			input:    "[![GitHub stars](https://img.shields.io/github/stars/guttermonk/bleamd)](https://github.com/guttermonk/bleamd/stargazers)",
			contains: "[stars: guttermonk/bleamd]",
		},
		{
			name:     "Standalone badge",
			input:    "![Build](https://img.shields.io/badge/build-passing-green)",
			contains: "[build: passing]",
		},
	}

	config := DefaultConfig()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processBadges(tt.input, config)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected output to contain %q, got %q", tt.contains, result)
			}
		})
	}
}
