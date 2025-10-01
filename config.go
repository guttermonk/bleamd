package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MichaelMure/go-term-markdown"
)

// Config holds the configuration for bleamd
type Config struct {
	Colors     ColorConfig     `json:"colors"`
	Keybindings KeybindingConfig `json:"keybindings"`
}

// KeybindingConfig holds custom keybinding settings
type KeybindingConfig struct {
	// Navigation keys
	ScrollUp       []string `json:"scroll_up"`
	ScrollDown     []string `json:"scroll_down"`
	ScrollLeft     []string `json:"scroll_left"`
	ScrollRight    []string `json:"scroll_right"`
	PageUp         []string `json:"page_up"`
	PageDown       []string `json:"page_down"`
	GoToTop        []string `json:"go_to_top"`
	GoToBottom     []string `json:"go_to_bottom"`
	
	// Search keys
	StartSearch    []string `json:"start_search"`
	NextMatch      []string `json:"next_match"`
	PrevMatch      []string `json:"prev_match"`
	ClearSearch    []string `json:"clear_search"`
	
	// General keys
	Quit           []string `json:"quit"`
	ShowHelp       []string `json:"show_help"`
	ToggleMouse    []string `json:"toggle_mouse"`
}

// ColorConfig holds color settings for markdown elements
type ColorConfig struct {
	// Headings
	Heading1       string `json:"heading1"`
	Heading2       string `json:"heading2"`
	Heading3       string `json:"heading3"`
	Heading4       string `json:"heading4"`
	Heading5       string `json:"heading5"`
	Heading6       string `json:"heading6"`
	
	// Text elements
	Bold           string `json:"bold"`
	Italic         string `json:"italic"`
	Strikethrough  string `json:"strikethrough"`
	Link           string `json:"link"`
	LinkURL        string `json:"link_url"`
	
	// Code
	Code           string `json:"code"`
	CodeBlock      string `json:"code_block"`
	CodeBlockBg    string `json:"code_block_bg"`
	
	// Lists
	ListMarker     string `json:"list_marker"`
	TaskChecked    string `json:"task_checked"`
	TaskUnchecked  string `json:"task_unchecked"`
	
	// Quotes and tables
	BlockQuote     string `json:"blockquote"`
	TableHeader    string `json:"table_header"`
	TableRow       string `json:"table_row"`
	TableBorder    string `json:"table_border"`
	
	// Search highlighting (for our search feature)
	SearchCurrent  string `json:"search_current"`
	SearchMatch    string `json:"search_match"`
	
	// Status bar
	StatusBarText  string `json:"status_bar_text"`
	StatusBarBg    string `json:"status_bar_bg"`
	
	// UI elements
	SearchBoxBorder  string `json:"search_box_border"`
	HelpBoxBorder    string `json:"help_box_border"`
	HoveredLinkURL   string `json:"hovered_link_url"`
	
	// Hyperlinks
	HyperlinkText             string `json:"hyperlink_text"`
	HyperlinkUnderline        string `json:"hyperlink_underline"`
	HyperlinkHoveredUnderline string `json:"hyperlink_hovered_underline"`
}



// DefaultKeybindings returns the default keybinding configuration
func DefaultKeybindings() KeybindingConfig {
	return KeybindingConfig{
		// Navigation - supports both Vim and Colemak-DH
		ScrollUp:    []string{"k", "i", "Up", "C-p"},
		ScrollDown:  []string{"j", "e", "Down", "C-n"},
		ScrollLeft:  []string{"h", "Left"},
		ScrollRight: []string{"l", "o", "Right"},
		PageUp:      []string{"PageUp"},
		PageDown:    []string{"PageDown", "Space"},
		GoToTop:     []string{"g"},
		GoToBottom:  []string{"G"},
		
		// Search
		StartSearch: []string{"/", "C-f"},
		NextMatch:   []string{"n"},
		PrevMatch:   []string{"N"},
		ClearSearch: []string{"Escape"},
		
		// General
		Quit:         []string{"q", "C-c"},
		ShowHelp:     []string{"?"},
		ToggleMouse:  []string{"m"},
	}
}

// OneDarkConfig returns a config with One Dark theme
func OneDarkConfig() *Config {
	return &Config{
		Keybindings: DefaultKeybindings(),
		Colors: ColorConfig{
			// Headings - One Dark blue/purple shades
			Heading1:       "#61afef",
			Heading2:       "#c678dd",
			Heading3:       "#61afef",
			Heading4:       "#c678dd",
			Heading5:       "#56b6c2",
			Heading6:       "#61afef",
			
			// Text elements
			Bold:           "#abb2bf",
			Italic:         "#98c379",
			Strikethrough:  "#5c6370",
			Link:           "#56b6c2",
			LinkURL:        "#61afef",
			
			// Code
			Code:           "#e5c07b",
			CodeBlock:      "#e5c07b",
			CodeBlockBg:    "#282c34",
			
			// Lists
			ListMarker:     "#d19a66",
			TaskChecked:    "#98c379",
			TaskUnchecked:  "#e06c75",
			
			// Quotes and tables
			BlockQuote:     "#5c6370",
			TableHeader:    "#e5c07b",
			TableRow:       "#abb2bf",
			TableBorder:    "#5c6370",
			
			// Search
			SearchCurrent:  "#d19a66", // Orange background for current match
			SearchMatch:    "#e5c07b", // Yellow background for other matches
			
			// Status bar
			StatusBarText:  "#5c6370", // Dark gray
			StatusBarBg:    "",        // Transparent
			
			// UI elements
			SearchBoxBorder: "#61afef", // Blue
			HelpBoxBorder:   "#c678dd", // Purple
			HoveredLinkURL:  "#56b6c2", // Cyan
			
			// Hyperlinks
			HyperlinkText:             "", // Use default link color
			HyperlinkUnderline:        "#56b6c2", // Cyan underline
			HyperlinkHoveredUnderline: "#e5c07b", // Yellow underline when hovered
		},
	}
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Keybindings: DefaultKeybindings(),
		Colors: ColorConfig{
			// Headings - blue shades
			Heading1:       "#00d7ff",
			Heading2:       "#00afff",
			Heading3:       "#0087ff",
			Heading4:       "#005fff",
			Heading5:       "#0037ff",
			Heading6:       "#001fff",
			
			// Text elements
			Bold:           "#ffffff",
			Italic:         "#87ff00",
			Strikethrough:  "#808080",
			Link:           "#00ffff",
			LinkURL:        "#0087af",
			
			// Code
			Code:           "#ffff00",
			CodeBlock:      "#d7ff00",
			CodeBlockBg:    "#262626",
			
			// Lists
			ListMarker:     "#ff8700",
			TaskChecked:    "#00ff00",
			TaskUnchecked:  "#ff0000",
			
			// Quotes and tables
			BlockQuote:     "#808080",
			TableHeader:    "#ffff00",
			TableRow:       "#ffffff",
			TableBorder:    "#808080",
			
			// Search
			SearchCurrent:  "#ff8700", // Orange background for current match
			SearchMatch:    "#ffff00", // Yellow background for other matches
			
			// Status bar
			StatusBarText:  "#ffffff", // White
			StatusBarBg:    "",        // Transparent
			
			// UI elements
			SearchBoxBorder: "#ff5fff", // Magenta
			HelpBoxBorder:   "#5f87d7", // Blue
			HoveredLinkURL:  "#00ffff", // Cyan
			
			// Hyperlinks
			HyperlinkText:             "", // Use default link color
			HyperlinkUnderline:        "#00ffff", // Cyan underline
			HyperlinkHoveredUnderline: "#ffff00", // Yellow underline when hovered
		},
	}
}

// LoadConfig loads configuration from the config file
func LoadConfig() (*Config, error) {
	configPath := getConfigPath()
	
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config file
		config := DefaultConfig()
		if err := config.Save(); err != nil {
			return config, nil // Return default config even if save fails
		}
		return config, nil
	}
	
	// Read config file
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return DefaultConfig(), fmt.Errorf("failed to read config file: %w", err)
	}
	
	// Parse JSON config
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return DefaultConfig(), fmt.Errorf("failed to parse config file: %w", err)
	}
	
	// Fill in any missing values with defaults
	config.fillDefaults()
	
	return &config, nil
}

// Save saves the configuration to file
func (c *Config) Save() error {
	configPath := getConfigPath()
	configDir := filepath.Dir(configPath)
	
	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	// Marshal config to JSON with indentation
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	// Write config file
	if err := ioutil.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	return nil
}

// fillDefaults fills in any missing configuration values with defaults
func (c *Config) fillDefaults() {
	defaults := DefaultConfig()
	
	// Fill in keybindings if missing
	if c.Keybindings.ScrollUp == nil { c.Keybindings.ScrollUp = defaults.Keybindings.ScrollUp }
	if c.Keybindings.ScrollDown == nil { c.Keybindings.ScrollDown = defaults.Keybindings.ScrollDown }
	if c.Keybindings.ScrollLeft == nil { c.Keybindings.ScrollLeft = defaults.Keybindings.ScrollLeft }
	if c.Keybindings.ScrollRight == nil { c.Keybindings.ScrollRight = defaults.Keybindings.ScrollRight }
	if c.Keybindings.PageUp == nil { c.Keybindings.PageUp = defaults.Keybindings.PageUp }
	if c.Keybindings.PageDown == nil { c.Keybindings.PageDown = defaults.Keybindings.PageDown }
	if c.Keybindings.GoToTop == nil { c.Keybindings.GoToTop = defaults.Keybindings.GoToTop }
	if c.Keybindings.GoToBottom == nil { c.Keybindings.GoToBottom = defaults.Keybindings.GoToBottom }
	if c.Keybindings.StartSearch == nil { c.Keybindings.StartSearch = defaults.Keybindings.StartSearch }
	if c.Keybindings.NextMatch == nil { c.Keybindings.NextMatch = defaults.Keybindings.NextMatch }
	if c.Keybindings.PrevMatch == nil { c.Keybindings.PrevMatch = defaults.Keybindings.PrevMatch }
	if c.Keybindings.ClearSearch == nil { c.Keybindings.ClearSearch = defaults.Keybindings.ClearSearch }
	if c.Keybindings.Quit == nil { c.Keybindings.Quit = defaults.Keybindings.Quit }
	if c.Keybindings.ShowHelp == nil { c.Keybindings.ShowHelp = defaults.Keybindings.ShowHelp }
	
	if c.Colors.Heading1 == "" { c.Colors.Heading1 = defaults.Colors.Heading1 }
	if c.Colors.Heading2 == "" { c.Colors.Heading2 = defaults.Colors.Heading2 }
	if c.Colors.Heading3 == "" { c.Colors.Heading3 = defaults.Colors.Heading3 }
	if c.Colors.Heading4 == "" { c.Colors.Heading4 = defaults.Colors.Heading4 }
	if c.Colors.Heading5 == "" { c.Colors.Heading5 = defaults.Colors.Heading5 }
	if c.Colors.Heading6 == "" { c.Colors.Heading6 = defaults.Colors.Heading6 }
	
	if c.Colors.Bold == "" { c.Colors.Bold = defaults.Colors.Bold }
	if c.Colors.Italic == "" { c.Colors.Italic = defaults.Colors.Italic }
	if c.Colors.Strikethrough == "" { c.Colors.Strikethrough = defaults.Colors.Strikethrough }
	if c.Colors.Link == "" { c.Colors.Link = defaults.Colors.Link }
	if c.Colors.LinkURL == "" { c.Colors.LinkURL = defaults.Colors.LinkURL }
	
	if c.Colors.Code == "" { c.Colors.Code = defaults.Colors.Code }
	if c.Colors.CodeBlock == "" { c.Colors.CodeBlock = defaults.Colors.CodeBlock }
	if c.Colors.CodeBlockBg == "" { c.Colors.CodeBlockBg = defaults.Colors.CodeBlockBg }
	
	if c.Colors.ListMarker == "" { c.Colors.ListMarker = defaults.Colors.ListMarker }
	if c.Colors.TaskChecked == "" { c.Colors.TaskChecked = defaults.Colors.TaskChecked }
	if c.Colors.TaskUnchecked == "" { c.Colors.TaskUnchecked = defaults.Colors.TaskUnchecked }
	
	if c.Colors.BlockQuote == "" { c.Colors.BlockQuote = defaults.Colors.BlockQuote }
	if c.Colors.TableHeader == "" { c.Colors.TableHeader = defaults.Colors.TableHeader }
	if c.Colors.TableRow == "" { c.Colors.TableRow = defaults.Colors.TableRow }
	if c.Colors.TableBorder == "" { c.Colors.TableBorder = defaults.Colors.TableBorder }
	
	if c.Colors.SearchCurrent == "" { c.Colors.SearchCurrent = defaults.Colors.SearchCurrent }
	if c.Colors.SearchMatch == "" { c.Colors.SearchMatch = defaults.Colors.SearchMatch }
	
	if c.Colors.SearchBoxBorder == "" { c.Colors.SearchBoxBorder = defaults.Colors.SearchBoxBorder }
	if c.Colors.HelpBoxBorder == "" { c.Colors.HelpBoxBorder = defaults.Colors.HelpBoxBorder }
	if c.Colors.HoveredLinkURL == "" { c.Colors.HoveredLinkURL = defaults.Colors.HoveredLinkURL }
	// If hyperlink_underline is empty, use the link text color
	if c.Colors.HyperlinkUnderline == "" { 
		if c.Colors.Link != "" {
			c.Colors.HyperlinkUnderline = c.Colors.Link
		} else {
			c.Colors.HyperlinkUnderline = defaults.Colors.Link
		}
	}
	if c.Colors.HyperlinkHoveredUnderline == "" { c.Colors.HyperlinkHoveredUnderline = defaults.Colors.HyperlinkHoveredUnderline }
	
	// Note: StatusBarText, StatusBarBg, and HyperlinkText can be empty (for defaults/transparent)
	// So we don't fill defaults for them if empty - empty is a valid value
}

// getConfigPath returns the path to the config file
func getConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory
		return "bleamd-config.json"
	}
	return filepath.Join(homeDir, ".config", "bleamd", "config.json")
}

// hexToANSI converts a hex color to the nearest ANSI 256 color
func hexToANSI(hex string) (int, error) {
	// Remove # prefix if present
	hex = strings.TrimPrefix(hex, "#")
	
	// Parse hex color
	if len(hex) != 6 {
		return 0, fmt.Errorf("invalid hex color: %s", hex)
	}
	
	r, err := strconv.ParseInt(hex[0:2], 16, 64)
	if err != nil {
		return 0, err
	}
	g, err := strconv.ParseInt(hex[2:4], 16, 64)
	if err != nil {
		return 0, err
	}
	b, err := strconv.ParseInt(hex[4:6], 16, 64)
	if err != nil {
		return 0, err
	}
	
	// Convert to ANSI 256 color
	// This is a simplified conversion - more sophisticated algorithms exist
	if r == g && g == b {
		// Grayscale
		if r < 8 {
			return 16, nil
		}
		if r > 248 {
			return 231, nil
		}
		return int(232 + ((r-8)/10)), nil
	}
	
	// Color cube (6x6x6)
	r = (r * 5) / 255
	g = (g * 5) / 255
	b = (b * 5) / 255
	
	return int(16 + (36 * r) + (6 * g) + b), nil
}

// GetANSIColor returns ANSI escape code for a hex color
func (c *ColorConfig) GetANSIColor(hex string) string {
	if hex == "" {
		return ""
	}
	
	colorCode, err := hexToANSI(hex)
	if err != nil {
		// Return default color on error
		return "\033[0m"
	}
	
	return fmt.Sprintf("\033[38;5;%dm", colorCode)
}

// GetANSIBackground returns ANSI escape code for background color
func (c *ColorConfig) GetANSIBackground(hex string) string {
	if hex == "" {
		return ""
	}
	
	colorCode, err := hexToANSI(hex)
	if err != nil {
		return ""
	}
	
	return fmt.Sprintf("\033[48;5;%dm", colorCode)
}

// GetMarkdownOptions returns markdown rendering options based on config
func (c *Config) GetMarkdownOptions() []markdown.Options {
	opts := []markdown.Options{
		markdown.WithImageDithering(markdown.DitheringWithBlocks),
	}
	
	// Note: The go-term-markdown library may not support all color customizations
	// We'll need to work with what's available in the API
	// This is a placeholder for when we integrate with the actual markdown renderer
	
	return opts
}

// ApplySearchHighlight applies search highlighting colors to text
func (c *Config) ApplySearchHighlight(text string, isCurrent bool) string {
	if isCurrent {
		// Current match - orange background
		bgColor := c.Colors.GetANSIBackground(c.Colors.SearchCurrent)
		return fmt.Sprintf("%s\033[30m%s\033[0m", bgColor, text)
	} else {
		// Other matches - yellow background
		bgColor := c.Colors.GetANSIBackground(c.Colors.SearchMatch)
		return fmt.Sprintf("%s\033[30m%s\033[0m", bgColor, text)
	}
}