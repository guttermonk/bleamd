package main

import (
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// OSC 8 hyperlink escape sequence format:
// \x1b]8;;URL\x1b\\TEXT\x1b]8;;\x1b\\

// addHyperlinks adds OSC 8 hyperlink escape sequences and underlines to rendered markdown
// The go-term-markdown library renders links as [text](url) with the URL in blue
// We need to find these patterns and replace them with OSC 8 hyperlinks
func addHyperlinks(rendered []byte, originalMarkdown string, config *Config, hoveredURL string) []byte {
	result := string(rendered)
	
	// The challenge: go-term-markdown renders links as [text](url) but wraps the URL
	// in ANSI color codes. We need to match this pattern accounting for ANSI codes.
	
	// ANSI code pattern: \x1b[...m or \x1b]...
	// We need to match: [text](\x1b[38;5;...mURL\x1b[0m optional-title)
	
	// Strategy: Find [ and match until ], then match ( and find the closing )
	// while accounting for ANSI codes
	
	// Match markdown links [text](url) where both text and url can contain ANSI codes
	// The rendered text looks like: "\x1b[32m• \x1b[0mFollow the [Hyprland wiki](\x1b[34mhttps://wiki.hyprland.org\x1b[0m)"
	// Problem: [ appears in ANSI sequences like \x1b[32m, so naive regex will match those first
	// Solution: Search for the ]( pattern which is unique to markdown links, then backtrack to find [
	
	// First, find all ]( patterns (these only appear in markdown links, not ANSI sequences)
	closeBracketPattern := regexp.MustCompile(`\]\(([^)]+)\)`)
	closeBracketMatches := closeBracketPattern.FindAllStringSubmatchIndex(result, -1)
	
	// For each ]( found, backtrack to find the matching [
	var validMatches [][]int
	for _, match := range closeBracketMatches {
		closeBracketPos := match[0] // Position of ]
		urlStart := match[2]
		urlEnd := match[3]
		
		// Backtrack from ] to find the matching [
		// We need to find [ that's not preceded by \x1b
		openBracketPos := -1
		for pos := closeBracketPos - 1; pos >= 0; pos-- {
			if result[pos] == '[' {
				// Check if this [ is part of an ANSI sequence (preceded by \x1b)
				if pos > 0 && result[pos-1] == '\x1b' {
					// Skip this [, it's part of \x1b[...m
					continue
				}
				// Found the opening [
				openBracketPos = pos
				break
			}
		}
		
		if openBracketPos >= 0 {
			// Valid match found
			textStart := openBracketPos + 1
			textEnd := closeBracketPos
			matchEnd := match[1]
			
			validMatches = append(validMatches, []int{
				openBracketPos, matchEnd,
				textStart, textEnd,
				urlStart, urlEnd,
			})
		}
	}
	
	// Process matches in reverse order to avoid position shifts
	for i := len(validMatches) - 1; i >= 0; i-- {
		match := validMatches[i]
		
		matchStart := match[0]
		matchEnd := match[1]
		textStart := match[2]
		textEnd := match[3]
		urlStart := match[4]
		urlEnd := match[5]
		
		textWithANSI := result[textStart:textEnd]
		urlPartWithANSI := result[urlStart:urlEnd]
		
		// Keep the text with ANSI codes intact for proper color rendering
		text := textWithANSI
		
		// Strip ANSI codes from URL part to get the actual URL
		url := stripANSI(urlPartWithANSI)
		url = strings.TrimSpace(url)
		
		// Remove any title from the URL (space + title)
		if idx := strings.Index(url, " "); idx != -1 {
			url = url[:idx]
		}
		
		// Validate it looks like a URL
		if !strings.Contains(url, "://") && !strings.HasPrefix(url, "mailto:") {
			// Not a URL, might be something else, don't convert
			continue
		}
		
		// Create OSC 8 hyperlink with underline
		// Format: \x1b]8;;URL\x1b\\TEXT\x1b]8;;\x1b\\
		// Important: The OSC 8 terminator MUST be \x1b\\ (ESC backslash)
		// We need to close the hyperlink properly to avoid the whole document becoming a link
		
		// Get underline color from config
		// ANSI has different codes for underline style:
		// \x1b[4m = underline
		// \x1b[4:3m = curly underline (not widely supported)
		// \x1b[58;5;COLORm = underline color (CSI 58 - widely supported in modern terminals)
		// \x1b[59m = default underline color
		var hyperlinked string
		
		// Check if this URL is being hovered
		isHovered := hoveredURL != "" && url == hoveredURL
		
		// Choose underline color based on hover state
		var underlineColor string
		if isHovered && config != nil && config.Colors.HyperlinkHoveredUnderline != "" {
			underlineColor = config.Colors.HyperlinkHoveredUnderline
		} else if config != nil && config.Colors.HyperlinkUnderline != "" {
			underlineColor = config.Colors.HyperlinkUnderline
		}
		
		if underlineColor != "" {
			if colorCode, err := hexToANSI(underlineColor); err == nil {
				// Use CSI 58 for underline color, CSI 4 for underline style
				// Set underline color before the OSC 8 sequence so it applies to the entire link
				// Format: <underline-color><underline-on><OSC8-start>text<OSC8-end><underline-off><underline-color-reset>
				hyperlinked = fmt.Sprintf("\x1b[58;5;%dm\x1b[4m\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\\x1b[59m\x1b[24m", colorCode, url, text)
			} else {
				// Fallback to default underline if color parsing fails
				hyperlinked = fmt.Sprintf("\x1b[4m\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\\x1b[24m", url, text)
			}
		} else {
			// Default underline (no color specified)
			hyperlinked = fmt.Sprintf("\x1b[4m\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\\x1b[24m", url, text)
		}
		
		// Replace the match with the hyperlinked version
		result = result[:matchStart] + hyperlinked + result[matchEnd:]
	}
	
	return []byte(result)
}

// stripANSI removes all ANSI escape codes from a string
func stripANSI(s string) string {
	plain, _ := stripANSIWithMapping(s)
	return plain
}

// url_encode encodes a string for use in OSC 8 parameters
func url_encode(s string) string {
	return url.QueryEscape(s)
}

// openURL opens a URL in the system's default browser
func openURL(url string) error {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported platform")
	}
	
	return cmd.Start()
}