package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// processBadges converts shields.io badge images into text representations
// This allows badges to be displayed in the terminal similar to how Grip displays them
func processBadges(markdown string, config *Config) string {
	// Match shields.io badge images in markdown format
	// Pattern: [![alt text](https://img.shields.io/...)](optional-link)
	
	// First, find standalone badges: ![alt](shield-url)
	// Second, find linked badges: [![alt](shield-url)](link-url)
	
	result := markdown
	
	// Pattern for linked badges: [![alt](badge-url)](link)
	linkedBadgePattern := regexp.MustCompile(`\[!\[[^\]]*\]\((https://img\.shields\.io/[^)]+)\)\]\(([^)]+)\)`)
	
	// Pattern for standalone badges: ![alt](badge-url)
	standaloneBadgePattern := regexp.MustCompile(`!\[[^\]]*\]\((https://img\.shields\.io/[^)]+)\)`)
	
	// Process linked badges first (to avoid matching them as standalone)
	linkedMatches := linkedBadgePattern.FindAllStringSubmatchIndex(result, -1)
	
	// Process in reverse order to avoid position shifts
	for i := len(linkedMatches) - 1; i >= 0; i-- {
		match := linkedMatches[i]
		matchStart := match[0]
		matchEnd := match[1]
		badgeURLStart := match[2]
		badgeURLEnd := match[3]
		linkURLStart := match[4]
		linkURLEnd := match[5]
		
		badgeURL := result[badgeURLStart:badgeURLEnd]
		linkURL := result[linkURLStart:linkURLEnd]
		
		// Parse the badge to extract label and message
		label, message, color := parseShieldsBadge(badgeURL)
		
		// Create a text representation as a clickable link
		// Format: [label: message](link)
		textBadge := fmt.Sprintf("[%s](%s)", formatBadgeText(label, message, color, config), linkURL)
		
		result = result[:matchStart] + textBadge + result[matchEnd:]
	}
	
	// Process standalone badges
	standaloneMatches := standaloneBadgePattern.FindAllStringSubmatchIndex(result, -1)
	
	for i := len(standaloneMatches) - 1; i >= 0; i-- {
		match := standaloneMatches[i]
		matchStart := match[0]
		matchEnd := match[1]
		badgeURLStart := match[2]
		badgeURLEnd := match[3]
		
		badgeURL := result[badgeURLStart:badgeURLEnd]
		
		// Parse the badge to extract label and message
		label, message, color := parseShieldsBadge(badgeURL)
		
		// Create a text representation
		// Format: [label: message]
		textBadge := formatBadgeText(label, message, color, config)
		
		result = result[:matchStart] + textBadge + result[matchEnd:]
	}
	
	return result
}

// parseShieldsBadge extracts label, message, and color from a shields.io badge URL
func parseShieldsBadge(badgeURL string) (label, message, color string) {
	// Parse URL
	parsedURL, err := url.Parse(badgeURL)
	if err != nil {
		return "badge", "", ""
	}
	
	// Get path and query
	path := strings.TrimPrefix(parsedURL.Path, "/")
	query := parsedURL.Query()
	
	// Different shields.io URL patterns:
	// 1. /badge/<label>-<message>-<color>
	// 2. /github/license/<user>/<repo>
	// 3. /github/stars/<user>/<repo>
	
	// Check if it's a GitHub-specific badge
	if strings.HasPrefix(path, "github/license/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 4 {
			// GitHub license badge
			user := parts[2]
			repo := parts[3]
			// Remove .svg extension if present
			repo = strings.TrimSuffix(repo, ".svg")
			label = "license"
			message = fmt.Sprintf("%s/%s", user, repo)
			color = query.Get("color")
			if color == "" {
				color = "blue"
			}
			return
		}
	}
	
	if strings.HasPrefix(path, "github/stars/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 4 {
			// GitHub stars badge
			user := parts[2]
			repo := parts[3]
			label = "stars"
			message = fmt.Sprintf("%s/%s", user, repo)
			color = query.Get("color")
			if color == "" {
				color = "yellow"
			}
			return
		}
	}
	
	// Static badge pattern: /badge/<label>-<message>-<color>
	if strings.HasPrefix(path, "badge/") {
		badgeInfo := strings.TrimPrefix(path, "badge/")
		
		// The format is label-message-color, but labels and messages can contain dashes
		// We need to find the last dash (color) and second-to-last dash (message)
		parts := strings.Split(badgeInfo, "-")
		
		if len(parts) >= 3 {
			// Last part is color
			color = parts[len(parts)-1]
			// Second to last is message
			message = parts[len(parts)-2]
			// Everything else is label
			label = strings.Join(parts[:len(parts)-2], "-")
		} else if len(parts) == 2 {
			label = parts[0]
			message = parts[1]
			color = ""
		} else if len(parts) == 1 {
			label = parts[0]
			message = ""
			color = ""
		}
		
		// URL decode the parts
		label = urlDecode(label)
		message = urlDecode(message)
		
		return
	}
	
	// Fallback: use the path as label
	label = path
	message = ""
	color = ""
	
	return
}

// urlDecode decodes URL-encoded strings, handling special characters
func urlDecode(s string) string {
	// Replace URL-encoded characters
	s = strings.ReplaceAll(s, "%20", " ")
	s = strings.ReplaceAll(s, "%2F", "/")
	s = strings.ReplaceAll(s, "%2D", "-")
	s = strings.ReplaceAll(s, "%5F", "_")
	
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// formatBadgeText formats the badge as colored text
func formatBadgeText(label, message, color string, config *Config) string {
	// Format: [label: message] or just [label] if no message
	
	var text string
	if message != "" && label != "" {
		text = fmt.Sprintf("[%s: %s]", label, message)
	} else if label != "" {
		text = fmt.Sprintf("[%s]", label)
	} else {
		text = "[badge]"
	}
	
	return text
}
