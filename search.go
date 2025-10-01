package main

import (
	"fmt"
	"regexp"
	"strings"
)

// SearchState manages the state of the search feature
type SearchState struct {
	active        bool
	term          string
	matches       []SearchMatch
	currentIndex  int
	caseSensitive bool
	config        *Config
}

// SearchMatch represents a single search match
type SearchMatch struct {
	lineNumber    int
	column        int  // Column in the plain text (without ANSI codes)
	originalColumn int  // Column in the original text (with ANSI codes)
	text          string
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// NewSearchState creates a new search state
func NewSearchState(config *Config) *SearchState {
	return &SearchState{
		active:        false,
		term:          "",
		matches:       []SearchMatch{},
		currentIndex:  -1,
		caseSensitive: false,
		config:        config,
	}
}

// Clear resets the search state
func (s *SearchState) Clear() {
	s.active = false
	s.term = ""
	s.matches = []SearchMatch{}
	s.currentIndex = -1
}

// SetTerm sets the search term and performs the search
func (s *SearchState) SetTerm(term string, content string) {
	s.term = term
	s.findAllMatches(content)
	if len(s.matches) > 0 {
		s.currentIndex = 0
	}
}

// stripANSI removes ANSI escape codes from a string and returns a mapping of
// plain text positions to original positions
func stripANSIWithMapping(s string) (plainText string, posMap []int) {
	posMap = make([]int, 0, len(s))
	plainBytes := make([]byte, 0, len(s))
	
	i := 0
	for i < len(s) {
		if i < len(s)-1 && s[i] == '\x1b' {
			if s[i+1] == '[' {
				// Found CSI escape sequence (e.g., \x1b[34m), skip it
				j := i + 2
				for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == ';') {
					j++
				}
				if j < len(s) {
					j++ // Skip the final letter
				}
				i = j
			} else if s[i+1] == ']' {
				// Found OSC escape sequence (e.g., \x1b]8;;URL\x1b\\), skip it
				j := i + 2
				// Skip until we find the string terminator \x1b\\ or BEL (\x07)
				for j < len(s) {
					if j < len(s)-1 && s[j] == '\x1b' && s[j+1] == '\\' {
						j += 2
						break
					} else if s[j] == '\x07' {
						j++
						break
					}
					j++
				}
				i = j
			} else {
				plainBytes = append(plainBytes, s[i])
				posMap = append(posMap, i)
				i++
			}
		} else {
			plainBytes = append(plainBytes, s[i])
			posMap = append(posMap, i)
			i++
		}
	}
	
	return string(plainBytes), posMap
}

// findAllMatches finds all matches in the content
func (s *SearchState) findAllMatches(content string) {
	s.matches = []SearchMatch{}
	if s.term == "" {
		return
	}

	lines := strings.Split(content, "\n")
	searchTerm := s.term
	
	if !s.caseSensitive {
		searchTerm = strings.ToLower(searchTerm)
	}

	for lineNum, line := range lines {
		// Strip ANSI codes for searching
		plainLine, posMap := stripANSIWithMapping(line)
		
		searchLine := plainLine
		if !s.caseSensitive {
			searchLine = strings.ToLower(plainLine)
		}

		// Find all occurrences in this line
		index := 0
		for {
			pos := strings.Index(searchLine[index:], searchTerm)
			if pos == -1 {
				break
			}
			
			actualPos := index + pos
			
			// Map plain text position to original position
			originalPos := 0
			if actualPos < len(posMap) {
				originalPos = posMap[actualPos]
			}
			
			// Extract the actual text from the plain line
			matchText := ""
			if actualPos+len(searchTerm) <= len(plainLine) {
				matchText = plainLine[actualPos : actualPos+len(searchTerm)]
			}
			
			s.matches = append(s.matches, SearchMatch{
				lineNumber:     lineNum,
				column:         actualPos,
				originalColumn: originalPos,
				text:           matchText,
			})
			
			index = actualPos + len(searchTerm)
		}
	}
}

// NextMatch moves to the next match
func (s *SearchState) NextMatch() (SearchMatch, bool) {
	if len(s.matches) == 0 {
		return SearchMatch{}, false
	}

	s.currentIndex = (s.currentIndex + 1) % len(s.matches)
	return s.matches[s.currentIndex], true
}

// PrevMatch moves to the previous match
func (s *SearchState) PrevMatch() (SearchMatch, bool) {
	if len(s.matches) == 0 {
		return SearchMatch{}, false
	}

	s.currentIndex--
	if s.currentIndex < 0 {
		s.currentIndex = len(s.matches) - 1
	}
	return s.matches[s.currentIndex], true
}

// GetCurrentMatch returns the current match
func (s *SearchState) GetCurrentMatch() (SearchMatch, bool) {
	if s.currentIndex < 0 || s.currentIndex >= len(s.matches) {
		return SearchMatch{}, false
	}
	return s.matches[s.currentIndex], true
}

// GetMatchCount returns the total number of matches
func (s *SearchState) GetMatchCount() int {
	return len(s.matches)
}

// GetStatusText returns status text for the search
func (s *SearchState) GetStatusText() string {
	if s.term == "" {
		return ""
	}
	
	if len(s.matches) == 0 {
		return fmt.Sprintf("No matches for: %s", s.term)
	}
	
	return fmt.Sprintf("Match %d of %d: %s", s.currentIndex+1, len(s.matches), s.term)
}

// HighlightContent highlights search matches in the content
func (s *SearchState) HighlightContent(content []byte) []byte {
	if s.term == "" || len(s.matches) == 0 {
		return content
	}

	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")
	
	// Create a map of line numbers to matches for efficient lookup
	lineMatches := make(map[int][]SearchMatch)
	for _, match := range s.matches {
		lineMatches[match.lineNumber] = append(lineMatches[match.lineNumber], match)
	}
	
	// Process each line that has matches
	for lineNum, matches := range lineMatches {
		if lineNum >= len(lines) {
			continue
		}
		
		line := lines[lineNum]
		
		// Strip ANSI codes to find match positions in plain text
		plainLine, posMap := stripANSIWithMapping(line)
		
		// Sort matches by column position
		// (they should already be sorted, but let's be safe)
		
		// Build highlighted line by inserting highlights at correct positions in plain text
		// then map back to original with ANSI codes
		var newLine strings.Builder
		plainPos := 0
		
		// Process matches in order by column position  
		for _, match := range matches {
			isCurrentMatch := false
			// Check if this match is the current one
			for j, m := range s.matches {
				if j == s.currentIndex && m.lineNumber == match.lineNumber && m.column == match.column {
					isCurrentMatch = true
					break
				}
			}
			
			// Add text before the match (from plain text)
			if match.column > plainPos {
				// Find the original text from plainPos to match.column
				if plainPos < len(posMap) && match.column <= len(posMap) {
					startOrig := posMap[plainPos]
					endOrig := posMap[match.column-1] + 1
					if match.column < len(posMap) {
						endOrig = posMap[match.column]
					}
					if startOrig < len(line) && endOrig <= len(line) {
						newLine.WriteString(line[startOrig:endOrig])
					}
				}
			}
			
			// Add highlighted match text (from plain text)
			matchEndPos := match.column + len(s.term)
			if matchEndPos <= len(plainLine) {
				matchText := plainLine[match.column:matchEndPos]
				
				if isCurrentMatch {
					// Current match - orange background (214)
					if s.config != nil {
						newLine.WriteString(s.config.ApplySearchHighlight(matchText, true))
					} else {
						// Fallback: orange background with black text
						newLine.WriteString("\033[48;5;214m\033[30m")
						newLine.WriteString(matchText)
						newLine.WriteString("\033[0m")
					}
				} else {
					// Other matches - yellow background (226)
					if s.config != nil {
						newLine.WriteString(s.config.ApplySearchHighlight(matchText, false))
					} else {
						// Fallback: yellow background with black text
						newLine.WriteString("\033[48;5;226m\033[30m")
						newLine.WriteString(matchText)
						newLine.WriteString("\033[0m")
					}
				}
			}
			
			plainPos = matchEndPos
		}
		
		// Add any remaining text after the last match
		if plainPos < len(plainLine) && plainPos < len(posMap) {
			startOrig := posMap[plainPos]
			if startOrig < len(line) {
				newLine.WriteString(line[startOrig:])
			}
		}
		
		lines[lineNum] = newLine.String()
	}
	
	return []byte(strings.Join(lines, "\n"))
}

// HandleSearchInput is no longer needed with Bubble Tea
// Input handling is done in the main Update method

// ToggleCaseSensitive toggles case-sensitive search
func (s *SearchState) ToggleCaseSensitive() {
	s.caseSensitive = !s.caseSensitive
}