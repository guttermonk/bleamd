package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/MichaelMure/go-term-markdown"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
)

const padding = 4

func main() {
	if len(os.Args) >= 2 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		printVersion()
		return
	}

	if len(os.Args) >= 2 && (os.Args[1] == "--init-config") {
		theme := "default"
		if len(os.Args) >= 3 {
			theme = os.Args[2]
		}
		initConfig(theme)
		return
	}

	if len(os.Args) >= 2 && (os.Args[1] == "--config-path") {
		fmt.Printf("Config file location: %s\n", getConfigPath())
		return
	}

	var content []byte

	switch len(os.Args) {
	case 1:
		if isatty.IsTerminal(os.Stdin.Fd()) {
			exitError(fmt.Errorf("usage: %s <file.md>", os.Args[0]))
		}
		data, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			exitError(errors.Wrap(err, "error while reading STDIN"))
		}
		content = data
	case 2:
		data, err := ioutil.ReadFile(os.Args[1])
		if err != nil {
			exitError(errors.Wrap(err, "error while reading file"))
		}
		err = os.Chdir(path.Dir(os.Args[1]))
		if err != nil {
			exitError(err)
		}
		content = data

	default:
		exitError(fmt.Errorf("only one file is supported"))
	}

	model := newModel(content)
	
	// Use default mouse mode (button clicks only) to allow text selection
	// WithMouseAllMotion() would capture all mouse events and prevent selection
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		exitError(errors.Wrap(err, "error starting the interactive UI"))
	}
}

func exitError(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func initConfig(theme string) {
	var config *Config
	
	switch theme {
	case "onedark", "one-dark":
		config = OneDarkConfig()
	case "default":
		config = DefaultConfig()
	default:
		fmt.Printf("Unknown theme: %s\n", theme)
		fmt.Println("Available themes: default, onedark")
		os.Exit(1)
	}
	
	configPath := getConfigPath()
	
	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config file already exists at: %s\n", configPath)
		fmt.Println("To regenerate, please delete the existing file first.")
		return
	}
	
	// Save the config
	if err := config.Save(); err != nil {
		exitError(fmt.Errorf("failed to create config file: %w", err))
	}
	
	fmt.Printf("Created %s theme config file at: %s\n", theme, configPath)
	fmt.Println("You can now edit this file to customize colors and keybindings.")
	fmt.Println("\nExample color values:")
	fmt.Println("  \"#ff0000\" - Red")
	fmt.Println("  \"#00ff00\" - Green")
	fmt.Println("  \"#0000ff\" - Blue")
	fmt.Println("  \"#ffff00\" - Yellow")
	fmt.Println("  \"#ff00ff\" - Magenta")
	fmt.Println("  \"#00ffff\" - Cyan")
}

type model struct {
	content         []byte
	raw             string
	width           int
	height          int
	xOffset         int
	yOffset         int
	lines           int
	renderedContent []byte
	
	// search state
	search       *SearchState
	searchActive bool
	searchInput  string
	
	// help state
	helpActive bool
	
	// configuration
	config *Config
	
	// hyperlink tracking for hover
	linkPositions []linkPosition
	hoveredURL    string
	
	// styles
	styles struct {
		helpBox   lipgloss.Style
		searchBox lipgloss.Style
		statusBar lipgloss.Style
	}
	
	// mode tracking for status bar
	mode string
	
	// mouse capture mode - toggleable for text selection
	mouseCaptureEnabled bool
}

func newModel(content []byte) model {
	config, err := LoadConfig()
	if err != nil {
		config = DefaultConfig()
	}
	
	m := model{
		content:             content,
		raw:                 string(content),
		width:               80, // Default width, will be updated on first WindowSizeMsg
		search:              NewSearchState(config),
		config:              config,
		mode:                "reading",
		mouseCaptureEnabled: true, // Start with mouse capture enabled for hover
	}
	
	// Initial render with default width
	m.renderedContent = m.render()
	// Count lines
	lineCount := 0
	for _, b := range m.renderedContent {
		if b == '\n' {
			lineCount++
		}
	}
	m.lines = lineCount
	
	// Initialize styles
	// Initialize help box style with configurable border color
	helpBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2)
	if config.Colors.HelpBoxBorder != "" {
		if colorCode, err := hexToANSI(config.Colors.HelpBoxBorder); err == nil {
			helpBoxStyle = helpBoxStyle.BorderForeground(lipgloss.Color(fmt.Sprintf("%d", colorCode)))
		}
	}
	m.styles.helpBox = helpBoxStyle
	
	// Initialize search box style with configurable border color  
	searchBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1)
	if config.Colors.SearchBoxBorder != "" {
		if colorCode, err := hexToANSI(config.Colors.SearchBoxBorder); err == nil {
			searchBoxStyle = searchBoxStyle.BorderForeground(lipgloss.Color(fmt.Sprintf("%d", colorCode)))
		}
	}
	m.styles.searchBox = searchBoxStyle
	
	m.styles.statusBar = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	
	return m
}

func (m model) Init() tea.Cmd {
	// Start with full mouse tracking enabled (for hover effects)
	// User can press 'm' to toggle and enable text selection
	return tea.EnableMouseAllMotion
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Re-render content with new width
		if len(m.raw) > 0 {
			m.renderedContent = m.render()
			// Count lines
			lineCount := 0
			for _, b := range m.renderedContent {
				if b == '\n' {
					lineCount++
				}
			}
			m.lines = lineCount
			
			// Update link positions for the current view
			m = m.updateLinkPositions()
		}
		return m, nil
		
	case tea.MouseMsg:
		return m.handleMouseMsg(msg)
		
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}
	
	return m, nil
}

func (m model) updateLinkPositions() model {
	// DEBUG
	f, _ := os.Create("/tmp/mdrs_update_debug.txt")
	if f != nil {
		fmt.Fprintf(f, "updateLinkPositions called\n")
		fmt.Fprintf(f, "  width=%d, height=%d\n", m.width, m.height)
		fmt.Fprintf(f, "  yOffset=%d, xOffset=%d\n", m.yOffset, m.xOffset)
	}
	
	// Replicate the View() logic to get visible content and extract link positions
	content := m.renderedContent
	if m.search.term != "" {
		content = m.search.HighlightContent(content)
	}
	
	lines := strings.Split(string(content), "\n")
	
	if f != nil {
		fmt.Fprintf(f, "  total lines=%d\n", len(lines))
	}
	
	// Calculate visible area (same logic as View())
	visibleHeight := m.height
	visibleHeight -= 1 // Status bar
	if m.searchActive {
		visibleHeight -= 3
	}
	if m.search.term != "" {
		visibleHeight -= 1
	}
	
	// Apply vertical scrolling
	startLine := m.yOffset
	endLine := startLine + visibleHeight
	
	if len(lines) == 0 {
		lines = []string{""}
	}
	
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine >= len(lines) {
		startLine = len(lines) - 1
	}
	if startLine < 0 {
		startLine = 0
	}
	if endLine < startLine {
		endLine = startLine
	}
	
	visibleLines := lines[startLine:endLine]
	
	// Apply horizontal scrolling
	for i, line := range visibleLines {
		if m.xOffset < len(line) {
			visibleLines[i] = line[m.xOffset:]
		} else {
			visibleLines[i] = ""
		}
	}
	
	result := strings.Join(visibleLines, "\n")
	
	// Extract link positions from visible content
	m.linkPositions = m.extractLinkPositions(result)
	
	if f != nil {
		fmt.Fprintf(f, "  extracted %d link positions\n", len(m.linkPositions))
		for i, link := range m.linkPositions {
			fmt.Fprintf(f, "    Link %d: %q at (%d,%d) width=%d\n", i, link.text, link.x, link.y, link.width)
		}
		f.Close()
	}
	
	return m
}

func (m model) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Handle mouse wheel scrolling
	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button == tea.MouseButtonWheelUp {
			return m.scrollUp(), nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			return m.scrollDown(), nil
		}
	}
	
	// Check if mouse is hovering over any link
	previousHoveredURL := m.hoveredURL
	m.hoveredURL = ""
	
	for _, link := range m.linkPositions {
		// Check if mouse position is within link bounds
		if msg.X >= link.x && msg.X < link.x+link.width && msg.Y == link.y {
			m.hoveredURL = link.url
			
			// Handle click on link
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
				openURL(link.url)
			}
			break
		}
	}
	
	// If hover state changed, re-render to update underline colors
	if previousHoveredURL != m.hoveredURL {
		m.renderedContent = m.render()
		m = m.updateLinkPositions()
	}
	
	return m, nil
}

func (m model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.helpActive {
		m.helpActive = false
		m.mode = "reading"
		return m, nil
	}
	
	if m.searchActive {
		m.mode = "search"
		switch msg.String() {
		case "enter":
			return m.executeSearch()
		case "esc", "ctrl+c", "ctrl+g":
			return m.cancelSearch()
		case "backspace":
			if len(m.searchInput) > 0 {
				m.searchInput = m.searchInput[:len(m.searchInput)-1]
			}
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.searchInput += msg.String()
			}
			return m, nil
		}
	}
	
	// Update mode based on search state
	if m.search.term != "" {
		m.mode = "search-nav"
	} else {
		m.mode = "reading"
	}
	
	// Handle navigation keys based on config
	key := msg.String()
	
	// In search-nav mode, allow escape or q to exit and clear search
	if m.mode == "search-nav" {
		if key == "esc" || key == "escape" {
			return m.clearSearch(), nil
		}
		// Check if 'q' is pressed and it's not bound to quit (to avoid conflicts)
		if key == "q" && !m.isKeyInSlice(key, m.config.Keybindings.Quit) {
			return m.clearSearch(), nil
		}
	}
	
	// Check if key matches any configured keybinding
	if m.isKeyInSlice(key, m.config.Keybindings.ScrollUp) {
		return m.scrollUp(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.ScrollDown) {
		return m.scrollDown(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.ScrollLeft) {
		return m.scrollLeft(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.ScrollRight) {
		return m.scrollRight(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.PageUp) {
		return m.pageUp(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.PageDown) {
		return m.pageDown(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.GoToTop) {
		return m.goToTop(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.GoToBottom) {
		return m.goToBottom(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.StartSearch) {
		return m.startSearch(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.NextMatch) {
		return m.nextMatch(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.PrevMatch) {
		return m.prevMatch(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.ClearSearch) {
		return m.clearSearch(), nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.ShowHelp) {
		m.helpActive = true
		m.mode = "help"
		return m, nil
	}
	if m.isKeyInSlice(key, m.config.Keybindings.Quit) {
		return m, tea.Quit
	}
	
	// Toggle mouse capture mode
	if m.isKeyInSlice(key, m.config.Keybindings.ToggleMouse) {
		m.mouseCaptureEnabled = !m.mouseCaptureEnabled
		if m.mouseCaptureEnabled {
			return m, tea.EnableMouseAllMotion
		} else {
			// Disable all mouse motion tracking to allow text selection
			return m, tea.DisableMouse
		}
	}
	
	return m, nil
}

func (m model) isKeyInSlice(key string, keys []string) bool {
	for _, k := range keys {
		if key == k || key == strings.ToLower(k) {
			return true
		}
		// Handle special key mappings
		switch k {
		case "Up", "ArrowUp":
			if key == "up" {
				return true
			}
		case "Down", "ArrowDown":
			if key == "down" {
				return true
			}
		case "Left", "ArrowLeft":
			if key == "left" {
				return true
			}
		case "Right", "ArrowRight":
			if key == "right" {
				return true
			}
		case "PageUp", "PgUp":
			if key == "pgup" {
				return true
			}
		case "PageDown", "PgDn", "PageDn":
			if key == "pgdown" {
				return true
			}
		case "Space", " ":
			if key == " " {
				return true
			}
		}
	}
	return false
}

func (m model) renderStatusBar() string {
	// If hovering over a link, show the URL instead of keybindings
	if m.hoveredURL != "" {
		style := lipgloss.NewStyle().
			Width(m.width).
			Padding(0, 1)
		
		// Apply hovered link URL color if configured, otherwise use status bar text color
		if m.config.Colors.HoveredLinkURL != "" {
			if colorCode, err := hexToANSI(m.config.Colors.HoveredLinkURL); err == nil {
				style = style.Foreground(lipgloss.Color(fmt.Sprintf("%d", colorCode)))
			}
		} else if m.config.Colors.StatusBarText != "" {
			if colorCode, err := hexToANSI(m.config.Colors.StatusBarText); err == nil {
				style = style.Foreground(lipgloss.Color(fmt.Sprintf("%d", colorCode)))
			}
		}
		
		// Apply background color if configured
		if m.config.Colors.StatusBarBg != "" {
			if colorCode, err := hexToANSI(m.config.Colors.StatusBarBg); err == nil {
				style = style.Background(lipgloss.Color(fmt.Sprintf("%d", colorCode)))
			}
		}
		
		return style.Render("🔗 " + m.hoveredURL)
	}
	
	// Helper to format key lists (take first key only for brevity)
	firstKey := func(keys []string) string {
		if len(keys) > 0 {
			key := keys[0]
			// Handle special keys
			switch key {
			case "Up", "ArrowUp":
				return "↑"
			case "Down", "ArrowDown":
				return "↓"
			case "Left", "ArrowLeft":
				return "←"
			case "Right", "ArrowRight":
				return "→"
			case "PageUp", "PgUp":
				return "PgUp"
			case "PageDown", "PgDn", "PageDn":
				return "PgDn"
			case "Space", " ":
				return "Space"
			case "Escape":
				return "Esc"
			}
			// Handle Ctrl+key
			if strings.HasPrefix(key, "C-") {
				return "^" + strings.TrimPrefix(key, "C-")
			}
			return key
		}
		return ""
	}
	
	var items []string
	
	switch m.mode {
	case "reading":
		// Show mouse mode indicator
		mouseMode := "hover"
		if !m.mouseCaptureEnabled {
			mouseMode = "select"
		}
		items = []string{
			fmt.Sprintf("%s/%s scroll", firstKey(m.config.Keybindings.ScrollUp), firstKey(m.config.Keybindings.ScrollDown)),
			fmt.Sprintf("%s/%s page", firstKey(m.config.Keybindings.PageUp), firstKey(m.config.Keybindings.PageDown)),
			fmt.Sprintf("%s search", firstKey(m.config.Keybindings.StartSearch)),
			fmt.Sprintf("%s mouse:%s", firstKey(m.config.Keybindings.ToggleMouse), mouseMode),
			fmt.Sprintf("%s help", firstKey(m.config.Keybindings.ShowHelp)),
			fmt.Sprintf("%s quit", firstKey(m.config.Keybindings.Quit)),
		}
	case "search":
		items = []string{
			"Enter execute",
			"Esc cancel",
			"type to search...",
		}
	case "search-nav":
		items = []string{
			fmt.Sprintf("%s/%s scroll", firstKey(m.config.Keybindings.ScrollUp), firstKey(m.config.Keybindings.ScrollDown)),
			fmt.Sprintf("%s/%s match", firstKey(m.config.Keybindings.NextMatch), firstKey(m.config.Keybindings.PrevMatch)),
			fmt.Sprintf("%s clear", firstKey(m.config.Keybindings.ClearSearch)),
			fmt.Sprintf("%s help", firstKey(m.config.Keybindings.ShowHelp)),
			fmt.Sprintf("%s quit", firstKey(m.config.Keybindings.Quit)),
		}
	case "help":
		items = []string{
			"Press any key to close help",
		}
	}
	
	// Join items with separator
	statusText := strings.Join(items, " │ ")
	
	// Apply styling - full width with configurable colors
	style := lipgloss.NewStyle().
		Width(m.width).
		Padding(0, 1)
	
	// Apply text color if configured
	if m.config.Colors.StatusBarText != "" {
		if colorCode, err := hexToANSI(m.config.Colors.StatusBarText); err == nil {
			style = style.Foreground(lipgloss.Color(fmt.Sprintf("%d", colorCode)))
		}
	}
	
	// Apply background color if configured (empty = transparent)
	if m.config.Colors.StatusBarBg != "" {
		if colorCode, err := hexToANSI(m.config.Colors.StatusBarBg); err == nil {
			style = style.Background(lipgloss.Color(fmt.Sprintf("%d", colorCode)))
		}
	}
	
	return style.Render(statusText)
}

func (m model) View() string {
	// Get the content to display (needed even when help is active for background)
	content := m.renderedContent
	if m.search.term != "" {
		content = m.search.HighlightContent(content)
	}
	
	if m.helpActive {
		return m.renderHelp(content)
	}
	
	// Apply viewport scrolling
	lines := strings.Split(string(content), "\n")
	
	// Calculate visible area
	visibleHeight := m.height
	visibleHeight -= 1 // Always reserve space for status bar at bottom
	if m.searchActive {
		visibleHeight -= 3 // Reserve space for search input
	}
	if m.search.term != "" {
		visibleHeight -= 1 // Reserve space for search status (Match X of Y)
	}
	
	// Apply vertical scrolling
	startLine := m.yOffset
	endLine := startLine + visibleHeight
	
	// Handle empty content
	if len(lines) == 0 {
		lines = []string{""}
	}
	
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine >= len(lines) {
		startLine = len(lines) - 1
	}
	if startLine < 0 {
		startLine = 0
	}
	if endLine < startLine {
		endLine = startLine
	}
	
	visibleLines := lines[startLine:endLine]
	
	// Apply horizontal scrolling
	for i, line := range visibleLines {
		if m.xOffset < len(line) {
			visibleLines[i] = line[m.xOffset:]
		} else {
			visibleLines[i] = ""
		}
	}
	
	result := strings.Join(visibleLines, "\n")
	
	// Calculate how many lines we've used so far
	contentLines := len(visibleLines)
	
	// Calculate how much padding we need before search box and status bar
	searchBoxLines := 0
	if m.searchActive {
		searchBoxLines = 3 // Search box typically takes 3 lines with border
	}
	
	extraLines := 0
	if m.search.term != "" {
		extraLines = 1 // Reserve space for search status (Match X of Y)
	}
	
	totalUsedLines := contentLines + extraLines + searchBoxLines + 1 // +1 for status bar itself
	if totalUsedLines < m.height {
		paddingNeeded := m.height - totalUsedLines
		for i := 0; i < paddingNeeded; i++ {
			result += "\n"
		}
	}
	
	// Add search status if needed (Match X of Y) - after padding, before search box
	if m.search.term != "" {
		statusText := m.search.GetStatusText()
		if statusText != "" {
			// Apply same padding as status bar (0 vertical, 1 horizontal)
			searchStatusStyle := lipgloss.NewStyle().
				Width(m.width).
				Padding(0, 1)
			
			// Apply search status colors if configured
			if m.config.Colors.StatusBarText != "" {
				if colorCode, err := hexToANSI(m.config.Colors.StatusBarText); err == nil {
					searchStatusStyle = searchStatusStyle.Foreground(lipgloss.Color(fmt.Sprintf("%d", colorCode)))
				}
			}
			
			result += "\n" + searchStatusStyle.Render(statusText)
		}
	}
	
	// Add search input if active (after padding, before status bar)
	if m.searchActive {
		// Create outer container that spans full width to center the search box
		searchBox := m.styles.searchBox.
			Width(m.width - 6).
			Render("Search: " + m.searchInput)
		
		// Center it with an outer style
		centered := lipgloss.NewStyle().
			Width(m.width).
			Align(lipgloss.Center).
			Render(searchBox)
		
		result += "\n" + centered
	}
	
	// Always add status bar at bottom
	result += "\n" + m.renderStatusBar()
	
	return result
}

func (m model) extractLinkPositions(content string) []linkPosition {
	// Extract hyperlink URLs and their text positions WITHOUT modifying the content
	// This preserves OSC 8 sequences so terminals can recognize clickable links
	
	hyperlinkPattern := regexp.MustCompile(`\x1b\]8;;([^\x1b]+)\x1b\\((?:[^\x1b]|\x1b\[[0-9;]*m)+)\x1b\]8;;\x1b\\`)
	
	var links []linkPosition
	lines := strings.Split(content, "\n")
	
	// DEBUG
	f, _ := os.Create("/tmp/mdrs_extract_debug.txt")
	if f != nil {
		fmt.Fprintf(f, "extractLinkPositions called with %d lines\n", len(lines))
	}
	
	for y, line := range lines {
		matches := hyperlinkPattern.FindAllStringSubmatchIndex(line, -1)
		if f != nil && len(matches) > 0 {
			fmt.Fprintf(f, "Line %d has %d matches\n", y, len(matches))
			fmt.Fprintf(f, "  Raw line (first 200 chars): %q\n", line[:min(200, len(line))])
		}
		
		for _, match := range matches {
			if len(match) >= 6 {
				urlStart := match[2]
				urlEnd := match[3]
				textStart := match[4]
				textEnd := match[5]
				
				url := line[urlStart:urlEnd]
				text := line[textStart:textEnd]
				
				// Strip ANSI codes from text to get visible length
				visibleText := stripANSI(text)
				
				// Calculate X position by counting visible characters before the link
				// We need to strip ALL escape sequences from the portion before the link text
				beforeLink := line[:match[4]] // Get everything before the link text starts
				visibleBefore := stripAllEscapeSequences(beforeLink)
				x := len(visibleBefore)
				
				if f != nil {
					fmt.Fprintf(f, "  Found link: url=%s, text=%q, visibleText=%q\n", url, text, visibleText)
					fmt.Fprintf(f, "    beforeLink length=%d, visibleBefore=%q (len=%d)\n", len(beforeLink), visibleBefore, len(visibleBefore))
					fmt.Fprintf(f, "    Position: x=%d, y=%d, width=%d\n", x, y, len(visibleText))
				}
				
				links = append(links, linkPosition{
					url:    url,
					text:   visibleText,
					x:      x,
					y:      y,
					width:  len(visibleText),
				})
			}
		}
	}
	
	if f != nil {
		fmt.Fprintf(f, "\nTotal links extracted: %d\n", len(links))
		f.Close()
	}
	
	return links
}

func stripAllEscapeSequences(s string) string {
	// Remove all ANSI escape sequences AND OSC 8 sequences
	// OSC 8: \x1b]8;;URL\x1b\\
	osc8Pattern := regexp.MustCompile(`\x1b\]8;;[^\x1b]*\x1b\\`)
	s = osc8Pattern.ReplaceAllString(s, "")
	
	// ANSI codes: \x1b[...m
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	s = ansiPattern.ReplaceAllString(s, "")
	
	return s
}

type linkPosition struct {
	url   string
	text  string
	x     int
	y     int
	width int
}

func (m model) render() []byte {
	// Get options from config, plus required options
	opts := m.config.GetMarkdownOptions()

	// Calculate render width
	// The markdown library includes both link text AND URL in line length calculations,
	// but we convert to OSC 8 hyperlinks where only the link text is visible.
	// So we render at a wider width to prevent unnecessary wrapping.
	// Use 2x terminal width to give plenty of room for URLs
	renderWidth := (m.width - padding) * 2
	if renderWidth < 40 {
		renderWidth = 40
	}
	
	rendered := markdown.Render(m.raw, renderWidth, padding, opts...)
	
	// Add hyperlinks with underlines (pass hoveredURL for hover state)
	rendered = addHyperlinks(rendered, m.raw, m.config, m.hoveredURL)
	
	// Count lines
	lineCount := 0
	for _, b := range rendered {
		if b == '\n' {
			lineCount++
		}
	}
	
	// Update the model's line count (this is a bit of a hack since we can't modify m in this method)
	// We'll handle this in the View method instead
	return rendered
}

func (m model) renderHelp(backgroundContent []byte) string {
	// Render the full background view exactly as it would appear normally
	// This is simpler than trying to reconstruct it
	normalView := m.renderNormalView()
	bgLines := strings.Split(normalView, "\n")
	
	// Ensure we have exactly m.height lines
	for len(bgLines) < m.height {
		bgLines = append(bgLines, "")
	}
	if len(bgLines) > m.height {
		bgLines = bgLines[:m.height]
	}
	
	// Render the help box (no fixed height so it sizes to content)
	helpContent := m.buildHelpContent()
	helpBox := m.styles.helpBox.
		Width(60).
		Render(helpContent)
	
	helpLines := strings.Split(helpBox, "\n")
	
	// Calculate centered position for overlay
	helpHeight := len(helpLines)
	// The border adds to the width, so measure the actual rendered width
	// Use rune count for proper Unicode character counting
	helpWidth := 0
	for _, line := range helpLines {
		stripped := stripANSI(line)
		w := len([]rune(stripped)) // Count runes, not bytes
		if w > helpWidth {
			helpWidth = w
		}
	}
	
	// DEBUG
	f, _ := os.Create("/tmp/mdrs_help_debug.txt")
	if f != nil {
		fmt.Fprintf(f, "m.width=%d, m.height=%d\n", m.width, m.height)
		fmt.Fprintf(f, "helpWidth=%d, helpHeight=%d\n", helpWidth, helpHeight)
		fmt.Fprintf(f, "First help line: %q\n", helpLines[0])
		fmt.Fprintf(f, "First help line visible length: %d\n", len([]rune(stripANSI(helpLines[0]))))
		f.Close()
	}
	
	startY := (m.height - helpHeight) / 2
	startX := (m.width - helpWidth) / 2
	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}
	
	// DEBUG
	f, _ = os.OpenFile("/tmp/mdrs_help_debug.txt", os.O_APPEND|os.O_WRONLY, 0644)
	if f != nil {
		fmt.Fprintf(f, "startX=%d, startY=%d\n", startX, startY)
		f.Close()
	}
	
	// Overlay the help box onto the background
	for i, helpLine := range helpLines {
		y := startY + i
		if y >= 0 && y < len(bgLines) {
			bgLine := bgLines[y]
			
			// Build new line with overlay
			var result strings.Builder
			
			// Left part of background
			if startX > 0 {
				leftPart := truncateVisibleChars(bgLine, startX)
				result.WriteString(leftPart)
				// Pad if needed
				leftLen := len([]rune(stripANSI(leftPart)))
				if leftLen < startX {
					result.WriteString(strings.Repeat(" ", startX-leftLen))
				}
			}
			
			// Help box line
			result.WriteString(helpLine)
			
			// Right part of background
			helpVisibleLen := len([]rune(stripANSI(helpLine)))
			endX := startX + helpVisibleLen
			bgVisibleLen := len([]rune(stripANSI(bgLine)))
			if endX < bgVisibleLen {
				rightPart := skipVisibleChars(bgLine, endX)
				result.WriteString(rightPart)
			}
			
			bgLines[y] = result.String()
		}
	}
	
	return strings.Join(bgLines, "\n")
}

// renderNormalView renders the view without the help overlay
func (m model) renderNormalView() string {
	content := m.renderedContent
	if m.search.term != "" {
		content = m.search.HighlightContent(content)
	}
	
	// Apply viewport scrolling
	lines := strings.Split(string(content), "\n")
	
	// Calculate visible area
	visibleHeight := m.height
	visibleHeight -= 1 // Always reserve space for status bar at bottom
	if m.searchActive {
		visibleHeight -= 3 // Reserve space for search input
	}
	if m.search.term != "" {
		visibleHeight -= 1 // Reserve space for search status (Match X of Y)
	}
	
	// Apply vertical scrolling
	startLine := m.yOffset
	endLine := startLine + visibleHeight
	
	// Handle empty content
	if len(lines) == 0 {
		lines = []string{""}
	}
	
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine >= len(lines) {
		startLine = len(lines) - 1
	}
	if startLine < 0 {
		startLine = 0
	}
	if endLine < startLine {
		endLine = startLine
	}
	
	visibleLines := lines[startLine:endLine]
	
	// Apply horizontal scrolling
	for i, line := range visibleLines {
		if m.xOffset < len(line) {
			visibleLines[i] = line[m.xOffset:]
		} else {
			visibleLines[i] = ""
		}
	}
	
	result := strings.Join(visibleLines, "\n")
	
	// Calculate how many lines we've used so far
	contentLines := len(visibleLines)
	
	// Calculate how much padding we need before search box and status bar
	searchBoxLines := 0
	if m.searchActive {
		searchBoxLines = 3 // Search box typically takes 3 lines with border
	}
	
	extraLines := 0
	if m.search.term != "" {
		extraLines = 1 // Reserve space for search status (Match X of Y)
	}
	
	totalUsedLines := contentLines + extraLines + searchBoxLines + 1 // +1 for status bar itself
	if totalUsedLines < m.height {
		paddingNeeded := m.height - totalUsedLines
		for i := 0; i < paddingNeeded; i++ {
			result += "\n"
		}
	}
	
	// Add search status if needed (Match X of Y) - after padding, before search box
	if m.search.term != "" {
		statusText := m.search.GetStatusText()
		if statusText != "" {
			// Apply same padding as status bar (0 vertical, 1 horizontal)
			searchStatusStyle := lipgloss.NewStyle().
				Width(m.width).
				Padding(0, 1)
			
			// Apply search status colors if configured
			if m.config.Colors.StatusBarText != "" {
				if colorCode, err := hexToANSI(m.config.Colors.StatusBarText); err == nil {
					searchStatusStyle = searchStatusStyle.Foreground(lipgloss.Color(fmt.Sprintf("%d", colorCode)))
				}
			}
			
			result += "\n" + searchStatusStyle.Render(statusText)
		}
	}
	
	// Add search input if active (after padding, before status bar)
	if m.searchActive {
		// Create outer container that spans full width to center the search box
		searchBox := m.styles.searchBox.
			Width(m.width - 6).
			Render("Search: " + m.searchInput)
		
		// Center it with an outer style
		centered := lipgloss.NewStyle().
			Width(m.width).
			Align(lipgloss.Center).
			Render(searchBox)
		
		result += "\n" + centered
	}
	
	// Always add status bar at bottom
	result += "\n" + m.renderStatusBar()
	
	return result
}

// truncateVisibleChars returns the prefix of s up to n visible characters
func truncateVisibleChars(s string, n int) string {
	var result strings.Builder
	visibleCount := 0
	i := 0
	bytes := []byte(s)
	
	for i < len(bytes) && visibleCount < n {
		if bytes[i] == '\x1b' {
			// Start of escape sequence
			escStart := i
			i++
			if i < len(bytes) && bytes[i] == '[' {
				// ANSI escape (\x1b[...m)
				i++
				for i < len(bytes) && !((bytes[i] >= 'A' && bytes[i] <= 'Z') || (bytes[i] >= 'a' && bytes[i] <= 'z')) {
					i++
				}
				if i < len(bytes) {
					i++
				}
				result.Write(bytes[escStart:i])
			} else if i < len(bytes) && bytes[i] == ']' {
				// OSC escape (\x1b]...\x1b\\)
				i++
				for i < len(bytes)-1 {
					if bytes[i] == '\x1b' && i+1 < len(bytes) && bytes[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
				result.Write(bytes[escStart:i])
			} else {
				// Unknown escape, just write the ESC char
				result.WriteByte(bytes[escStart])
				i = escStart + 1
			}
		} else {
			// Regular character - decode UTF-8 rune
			r, size := decodeRuneInBytes(bytes[i:])
			result.WriteRune(r)
			i += size
			visibleCount++
		}
	}
	
	return result.String()
}

// decodeRuneInBytes decodes a single UTF-8 rune from bytes
func decodeRuneInBytes(b []byte) (rune, int) {
	if len(b) == 0 {
		return 0, 0
	}
	// Simple UTF-8 decoding
	if b[0] < 0x80 {
		return rune(b[0]), 1
	}
	// Convert to string and use built-in rune conversion
	s := string(b)
	if len(s) == 0 {
		return 0, 0
	}
	r := []rune(s)[0]
	return r, len(string(r))
}

// skipVisibleChars returns the suffix of s starting after n visible characters
func skipVisibleChars(s string, n int) string {
	visibleCount := 0
	i := 0
	bytes := []byte(s)
	var pendingEscapes strings.Builder
	
	for i < len(bytes) {
		if bytes[i] == '\x1b' {
			// Start of escape sequence
			escStart := i
			i++
			if i < len(bytes) && bytes[i] == '[' {
				// ANSI escape (\x1b[...m)
				i++
				for i < len(bytes) && !((bytes[i] >= 'A' && bytes[i] <= 'Z') || (bytes[i] >= 'a' && bytes[i] <= 'z')) {
					i++
				}
				if i < len(bytes) {
					i++
				}
				if visibleCount >= n {
					pendingEscapes.Write(bytes[escStart:i])
				}
			} else if i < len(bytes) && bytes[i] == ']' {
				// OSC escape (\x1b]...\x1b\\)
				i++
				for i < len(bytes)-1 {
					if bytes[i] == '\x1b' && i+1 < len(bytes) && bytes[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
				if visibleCount >= n {
					pendingEscapes.Write(bytes[escStart:i])
				}
			} else {
				// Unknown escape, just skip the ESC char
				i = escStart + 1
			}
		} else {
			if visibleCount >= n {
				return pendingEscapes.String() + string(bytes[i:])
			}
			// Decode UTF-8 rune to count properly
			_, size := decodeRuneInBytes(bytes[i:])
			i += size
			visibleCount++
		}
	}
	
	return ""
}

func (m model) buildHelpContent() string {
	var sb strings.Builder

	// Helper function to format key list
	formatKeys := func(keys []string) string {
		return strings.Join(keys, ", ")
	}

	// Navigation section
	sb.WriteString(" NAVIGATION\n")
	sb.WriteString(" ═══════════════════════════════════════════════\n")
	sb.WriteString(fmt.Sprintf("  %-20s Move up\n", formatKeys(m.config.Keybindings.ScrollUp)))
	sb.WriteString(fmt.Sprintf("  %-20s Move down\n", formatKeys(m.config.Keybindings.ScrollDown)))
	sb.WriteString(fmt.Sprintf("  %-20s Move left\n", formatKeys(m.config.Keybindings.ScrollLeft)))
	sb.WriteString(fmt.Sprintf("  %-20s Move right\n", formatKeys(m.config.Keybindings.ScrollRight)))
	sb.WriteString(fmt.Sprintf("  %-20s Page up\n", formatKeys(m.config.Keybindings.PageUp)))
	sb.WriteString(fmt.Sprintf("  %-20s Page down\n", formatKeys(m.config.Keybindings.PageDown)))
	sb.WriteString(fmt.Sprintf("  %-20s Go to top\n", formatKeys(m.config.Keybindings.GoToTop)))
	sb.WriteString(fmt.Sprintf("  %-20s Go to bottom\n", formatKeys(m.config.Keybindings.GoToBottom)))
	sb.WriteString("\n")

	// Search section
	sb.WriteString(" SEARCH\n")
	sb.WriteString(" ═══════════════════════════════════════════════\n")
	sb.WriteString(fmt.Sprintf("  %-20s Start search\n", formatKeys(m.config.Keybindings.StartSearch)))
	sb.WriteString(fmt.Sprintf("  %-20s Next match\n", formatKeys(m.config.Keybindings.NextMatch)))
	sb.WriteString(fmt.Sprintf("  %-20s Previous match\n", formatKeys(m.config.Keybindings.PrevMatch)))
	sb.WriteString(fmt.Sprintf("  %-20s Clear search\n", formatKeys(m.config.Keybindings.ClearSearch)))
	sb.WriteString("\n")

	// General section
	sb.WriteString(" GENERAL\n")
	sb.WriteString(" ═══════════════════════════════════════════════\n")
	sb.WriteString(fmt.Sprintf("  %-20s Show this help\n", formatKeys(m.config.Keybindings.ShowHelp)))
	sb.WriteString(fmt.Sprintf("  %-20s Quit\n", formatKeys(m.config.Keybindings.Quit)))
	sb.WriteString(fmt.Sprintf("  %-20s Toggle mouse mode\n", formatKeys(m.config.Keybindings.ToggleMouse)))
	sb.WriteString("\n")

	// Notes section
	sb.WriteString(" NOTES\n")
	sb.WriteString(" ═══════════════════════════════════════════════\n")
	sb.WriteString("  • Search is case-insensitive\n")
	sb.WriteString("  • While searching:\n")
	sb.WriteString("    - Enter to execute search\n")
	sb.WriteString("    - ESC or Ctrl+C to cancel\n")
	sb.WriteString("  • Mouse modes:\n")
	sb.WriteString("    - hover: Link hover/click, wheel scroll\n")
	sb.WriteString("    - select: Text selection enabled\n")


	return sb.String()
}

func (m model) startSearch() model {
	m.searchActive = true
	m.searchInput = ""
	return m
}

func (m model) executeSearch() (model, tea.Cmd) {
	searchText := strings.TrimSpace(m.searchInput)
	if searchText == "" {
		return m.cancelSearch()
	}

	// Perform the search
	m.search.SetTerm(searchText, string(m.renderedContent))
	
	// DEBUG: Write match count to file
	f, _ := os.Create("/tmp/mdrs_debug.txt")
	if f != nil {
		fmt.Fprintf(f, "Search term: %s\n", searchText)
		fmt.Fprintf(f, "Match count: %d\n", m.search.GetMatchCount())
		fmt.Fprintf(f, "Current index: %d\n", m.search.currentIndex)
		if match, ok := m.search.GetCurrentMatch(); ok {
			fmt.Fprintf(f, "Current match: line %d, col %d\n", match.lineNumber, match.column)
		}
		fmt.Fprintf(f, "yOffset before scroll: %d\n", m.yOffset)
		f.Close()
	}
	
	// If we found matches, scroll to the first one
	if match, ok := m.search.GetCurrentMatch(); ok {
		m = m.scrollToLine(match.lineNumber)
	}
	
	// DEBUG: Write yOffset after scroll
	f, _ = os.OpenFile("/tmp/mdrs_debug.txt", os.O_APPEND|os.O_WRONLY, 0644)
	if f != nil {
		fmt.Fprintf(f, "yOffset after scroll: %d\n", m.yOffset)
		f.Close()
	}

	m.searchActive = false
	m.searchInput = ""
	m.mode = "search-nav"
	m = m.updateLinkPositions()
	
	return m, nil
}

func (m model) cancelSearch() (model, tea.Cmd) {
	m.searchActive = false
	m.searchInput = ""
	m.search.Clear()
	m.mode = "reading"
	m = m.updateLinkPositions()
	
	return m, nil
}

func (m model) clearSearch() model {
	m.searchActive = false
	m.search.Clear()
	m.mode = "reading"
	
	return m.updateLinkPositions()
}

func (m model) nextMatch() model {
	if m.search.term == "" {
		return m
	}
	
	// DEBUG
	f, _ := os.OpenFile("/tmp/mdrs_debug.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if f != nil {
		fmt.Fprintf(f, "\n=== NextMatch called ===\n")
		fmt.Fprintf(f, "Before: currentIndex=%d, yOffset=%d\n", m.search.currentIndex, m.yOffset)
		f.Close()
	}
	
	if match, ok := m.search.NextMatch(); ok {
		// DEBUG
		f, _ := os.OpenFile("/tmp/mdrs_debug.txt", os.O_APPEND|os.O_WRONLY, 0644)
		if f != nil {
			fmt.Fprintf(f, "NextMatch returned: line %d, col %d\n", match.lineNumber, match.column)
			f.Close()
		}
		
		m = m.scrollToLine(match.lineNumber)
		
		// DEBUG
		f, _ = os.OpenFile("/tmp/mdrs_debug.txt", os.O_APPEND|os.O_WRONLY, 0644)
		if f != nil {
			fmt.Fprintf(f, "After scroll: yOffset=%d\n", m.yOffset)
			f.Close()
		}
	}
	
	return m.updateLinkPositions()
}

func (m model) prevMatch() model {
	if m.search.term == "" {
		return m
	}
	
	if match, ok := m.search.PrevMatch(); ok {
		m = m.scrollToLine(match.lineNumber)
	}
	
	return m.updateLinkPositions()
}

func (m model) scrollToLine(lineNumber int) model {
	// Calculate visible height (same as in View())
	visibleHeight := m.height
	visibleHeight -= 1 // Always reserve space for status bar at bottom
	if m.searchActive {
		visibleHeight -= 3 // Reserve space for search input
	}
	if m.search.term != "" {
		visibleHeight -= 1 // Reserve space for search status (Match X of Y)
	}
	
	// Try to center the match on screen
	targetOffset := lineNumber - visibleHeight/2
	
	// Clamp to valid range
	maxOffset := m.lines - visibleHeight + 1
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.yOffset = max(0, min(targetOffset, maxOffset))
	return m
}

func (m model) scrollUp() model {
	m.yOffset -= 1
	m.yOffset = max(m.yOffset, 0)
	return m.updateLinkPositions()
}

func (m model) scrollDown() model {
	m.yOffset += 1
	m.yOffset = min(m.yOffset, m.lines-m.height+1)
	m.yOffset = max(m.yOffset, 0)
	return m.updateLinkPositions()
}

func (m model) scrollLeft() model {
	m.xOffset -= 1
	m.xOffset = max(m.xOffset, 0)
	return m.updateLinkPositions()
}

func (m model) scrollRight() model {
	m.xOffset += 1
	return m.updateLinkPositions()
}

func (m model) pageUp() model {
	m.yOffset -= m.height / 2
	m.yOffset = max(m.yOffset, 0)
	return m.updateLinkPositions()
}

func (m model) pageDown() model {
	m.yOffset += m.height / 2
	m.yOffset = min(m.yOffset, m.lines-m.height+1)
	m.yOffset = max(m.yOffset, 0)
	return m.updateLinkPositions()
}

func (m model) goToTop() model {
	m.yOffset = 0
	return m.updateLinkPositions()
}

func (m model) goToBottom() model {
	m.yOffset = m.lines - m.height + 1
	m.yOffset = max(m.yOffset, 0)
	return m.updateLinkPositions()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// extractAllLinks parses markdown content and extracts all links
