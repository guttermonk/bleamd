package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/MichaelMure/go-term-markdown"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
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

	// transient toast notification (top-right corner)
	toastMsg string
	toastSeq int

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
		toastBox  lipgloss.Style
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
	
	// Initialize toast style with configurable border color
	toastBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1)
	if config.Colors.ToastBorder != "" {
		if colorCode, err := hexToANSI(config.Colors.ToastBorder); err == nil {
			toastBoxStyle = toastBoxStyle.BorderForeground(lipgloss.Color(fmt.Sprintf("%d", colorCode)))
		}
	}
	m.styles.toastBox = toastBoxStyle

	m.styles.statusBar = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginTop(1)

	return m
}

// Inset of the toast box from the right edge, and from the top edge when the
// toast is top-anchored.
const (
	toastMarginX   = 2
	toastMarginTop = 1
)

// toastGutterRows is the number of rows held back below the status bar.
//
// A bottom-anchored toast must put its *text* row on the status bar so the
// message lines up with the hovered URL. A bordered box is three rows tall, so
// it needs one row below that text for its bottom border. The status bar is
// otherwise the final row of the frame, so one row is reserved for it. The row
// is permanently reserved rather than claimed only while a toast is visible,
// which would make the status bar jump a line every time one appeared.
func (m model) toastGutterRows() int {
	if m.config.ToastAtBottom() {
		return 1
	}
	return 0
}

// toastExpiredMsg clears the toast. The sequence number identifies which toast
// the timer was started for, so a newer toast is not cleared by an older timer.
type toastExpiredMsg struct {
	seq int
}

// showToast sets the toast text and returns a command that clears it later.
// A configured duration of zero disables toasts entirely.
func (m *model) showToast(text string) tea.Cmd {
	d := m.config.ToastDuration()
	if d <= 0 {
		return nil
	}

	m.toastMsg = text
	m.toastSeq++
	seq := m.toastSeq
	return tea.Tick(d, func(time.Time) tea.Msg {
		return toastExpiredMsg{seq: seq}
	})
}

// renderToast composites the toast onto an already-rendered frame.
func (m model) renderToast(frame string) string {
	if m.toastMsg == "" {
		return frame
	}

	boxLines := strings.Split(m.styles.toastBox.Render(m.toastMsg), "\n")
	boxWidth := boxVisibleWidth(boxLines)
	lines := strings.Split(frame, "\n")

	// Bottom-anchored toasts sit flush with the final row, which is the status
	// bar, so the box's bottom border lines up with the hovered URL. The frame
	// is exactly m.height rows with nothing below it, so this is the lowest a
	// bordered box can go.
	startY := toastMarginTop
	rowsNeeded := len(boxLines) + toastMarginTop
	if m.config.ToastAtBottom() {
		startY = len(lines) - len(boxLines)
		rowsNeeded = len(boxLines)
	}

	// Skip the toast rather than clip it if the viewport cannot hold it.
	if boxWidth+toastMarginX > m.width || rowsNeeded > len(lines) {
		return frame
	}

	startX := m.width - boxWidth - toastMarginX
	if startX < 0 {
		startX = 0
	}

	overlayBox(lines, boxLines, startX, startY)
	return strings.Join(lines, "\n")
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
		
	case toastExpiredMsg:
		// Ignore timers belonging to a toast that has since been replaced
		if msg.seq == m.toastSeq {
			m.toastMsg = ""
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
	// Replicate the View() logic to get visible content and extract link positions
	content := m.renderedContent
	if m.search.term != "" {
		content = m.search.HighlightContent(content)
	}

	lines := strings.Split(string(content), "\n")

	// Calculate visible area (same logic as View())
	visibleHeight := m.height
	visibleHeight -= 1 // Status bar
	visibleHeight -= m.toastGutterRows()
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

	// Apply horizontal scrolling. Lines carry ANSI/OSC 8 escapes, so skip by
	// visible characters rather than bytes to avoid severing a sequence.
	for i, line := range visibleLines {
		if m.xOffset > 0 {
			visibleLines[i] = skipVisibleChars(line, m.xOffset)
		}
	}

	result := strings.Join(visibleLines, "\n")

	// Extract link positions from visible content
	m.linkPositions = m.extractLinkPositions(result)

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

	var cmd tea.Cmd
	for _, link := range m.linkPositions {
		// Check if mouse position is within link bounds
		if msg.X >= link.x && msg.X < link.x+link.width && msg.Y == link.y {
			m.hoveredURL = link.url

			// Handle click on link
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
				if err := openURL(link.url); err != nil {
					cmd = m.showToast("✗ Could not open link")
				} else {
					cmd = m.showToast("↗ Opening in browser…")
				}
			}
			break
		}
	}

	// If hover state changed, re-render to update underline colors
	if previousHoveredURL != m.hoveredURL {
		m.renderedContent = m.render()
		m = m.updateLinkPositions()
	}

	return m, cmd
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
		// Handle Ctrl+key format conversion from C-x to ctrl+x
		if strings.HasPrefix(k, "C-") {
			ctrlKey := "ctrl+" + strings.ToLower(strings.TrimPrefix(k, "C-"))
			if key == ctrlKey {
				return true
			}
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
			PaddingLeft(1).
			PaddingRight(1)
		
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
		PaddingLeft(1).
		PaddingRight(1)
	
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
	// The toast overlays whatever frame was composed, including the help popup.
	return m.renderToast(m.viewFrame())
}

func (m model) viewFrame() string {
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
	visibleHeight -= 2 // Reserve 2 blank lines above status bar
	visibleHeight -= m.toastGutterRows()
	if m.searchActive {
		visibleHeight -= 3 // Reserve space for search input
	}
	if m.search.term != "" {
		visibleHeight -= 1 // Reserve space for search status (Match X of Y)
	}
	// Note: We don't subtract 1 for the status bar itself because the status bar 
	// shares a line with the last newline from the content
	
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
	
	// Apply horizontal scrolling. Lines carry ANSI/OSC 8 escapes, so skip by
	// visible characters rather than bytes to avoid severing a sequence.
	for i, line := range visibleLines {
		if m.xOffset > 0 {
			visibleLines[i] = skipVisibleChars(line, m.xOffset)
		}
	}
	
	result := strings.Join(visibleLines, "\n")
	
	// Calculate how many lines we've used so far
	contentLines := len(visibleLines)
	
	// Add search status if needed (Match X of Y)
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
			contentLines++
		}
	}
	
	// Add search input if active
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
		contentLines += 3 // Search box takes 3 lines with border
	}
	
	// Calculate how much padding we need to push status bar to bottom
	// We need: contentLines + padding + 2 blank lines + status bar = m.height lines total
	// Which means: contentLines + padding + 2 + 1 = m.height
	// So: padding = m.height - contentLines - 3
	// BUT: we need one extra line because status bar shares the last line
	linesNeededForStatusBar := 2 + m.toastGutterRows() // 2 blank lines above, plus any reserved gutter below
	availableLinesForPadding := m.height - contentLines - linesNeededForStatusBar // status bar doesn't need a separate line count
	
	// Add padding to push status bar to bottom
	if availableLinesForPadding > 0 {
		for i := 0; i < availableLinesForPadding; i++ {
			result += "\n"
		}
	}
	
	// Add 2 blank lines above the status bar (margin top)
	result += "\n\n"
	
	// Add status bar, then any reserved gutter row beneath it
	result += m.renderStatusBar()
	result += strings.Repeat("\n", m.toastGutterRows())

	return result
}

func (m model) extractLinkPositions(content string) []linkPosition {
	// Extract hyperlink URLs and their text positions WITHOUT modifying the content
	// This preserves OSC 8 sequences so terminals can recognize clickable links
	
	hyperlinkPattern := regexp.MustCompile(`\x1b\]8;;([^\x1b]+)\x1b\\((?:[^\x1b]|\x1b\[[0-9;]*m)+)\x1b\]8;;\x1b\\`)
	
	var links []linkPosition
	lines := strings.Split(content, "\n")

	for y, line := range lines {
		matches := hyperlinkPattern.FindAllStringSubmatchIndex(line, -1)

		for _, match := range matches {
			if len(match) >= 6 {
				urlStart := match[2]
				urlEnd := match[3]
				textStart := match[4]
				textEnd := match[5]

				url := line[urlStart:urlEnd]
				text := line[textStart:textEnd]

				// Strip ANSI codes from text to get the visible label
				visibleText := stripANSI(text)

				// Calculate X position by measuring the display width of the
				// text before the link. Width, not byte length, so that CJK,
				// emoji and combining marks give correct hit boxes.
				beforeLink := line[:match[4]] // Get everything before the link text starts
				visibleBefore := stripAllEscapeSequences(beforeLink)
				x := runewidth.StringWidth(visibleBefore)

				links = append(links, linkPosition{
					url:   url,
					text:  visibleText,
					x:     x,
					y:     y,
					width: runewidth.StringWidth(visibleText),
				})
			}
		}
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
	
	// Process badges before rendering
	processedMarkdown := processBadges(m.raw, m.config)
	
	rendered := markdown.Render(processedMarkdown, renderWidth, padding, opts...)
	
	// Add hyperlinks with underlines (pass hoveredURL for hover state)
	rendered = addHyperlinks(rendered, processedMarkdown, m.config, m.hoveredURL)
	
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
	// Render the full background view WITHOUT search highlighting
	// Save the current search term and clear it temporarily
	savedSearchTerm := m.search.term
	m.search.term = ""
	
	normalView := m.renderNormalView()
	
	// Restore the search term
	m.search.term = savedSearchTerm
	
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
	helpWidth := boxVisibleWidth(helpLines)
	
	startY := (m.height - helpHeight) / 2
	startX := (m.width - helpWidth) / 2
	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}

	overlayBox(bgLines, helpLines, startX, startY)

	return strings.Join(bgLines, "\n")
}

// boxVisibleWidth returns the widest visible line in a rendered box, ignoring
// ANSI escapes.
func boxVisibleWidth(boxLines []string) int {
	width := 0
	for _, line := range boxLines {
		if w := visibleWidth(line); w > width {
			width = w
		}
	}
	return width
}

// overlayBox composites boxLines onto bgLines at (startX, startY), preserving
// the background to the left and right of the box. bgLines is modified in place.
func overlayBox(bgLines []string, boxLines []string, startX, startY int) {
	for i, boxLine := range boxLines {
		y := startY + i
		if y < 0 || y >= len(bgLines) {
			continue
		}
		bgLine := bgLines[y]

		var result strings.Builder

		// Left part of background
		if startX > 0 {
			leftPart := truncateVisibleChars(bgLine, startX)
			result.WriteString(leftPart)
			// Pad if the background line ends before the box starts, or if a
			// wide character was dropped at the cut
			if leftLen := visibleWidth(leftPart); leftLen < startX {
				result.WriteString(strings.Repeat(" ", startX-leftLen))
			}
		}

		// The box line itself
		result.WriteString(boxLine)

		// Right part of background
		endX := startX + visibleWidth(boxLine)
		if endX < visibleWidth(bgLine) {
			result.WriteString(skipVisibleChars(bgLine, endX))
		}

		bgLines[y] = result.String()
	}
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
	visibleHeight -= 2 // Reserve 2 blank lines above status bar
	visibleHeight -= m.toastGutterRows()
	if m.searchActive {
		visibleHeight -= 3 // Reserve space for search input
	}
	if m.search.term != "" {
		visibleHeight -= 1 // Reserve space for search status (Match X of Y)
	}
	// Note: We don't subtract 1 for the status bar itself because the status bar 
	// shares a line with the last newline from the content
	
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
	
	// Apply horizontal scrolling. Lines carry ANSI/OSC 8 escapes, so skip by
	// visible characters rather than bytes to avoid severing a sequence.
	for i, line := range visibleLines {
		if m.xOffset > 0 {
			visibleLines[i] = skipVisibleChars(line, m.xOffset)
		}
	}
	
	result := strings.Join(visibleLines, "\n")
	
	// Calculate how many lines we've used so far
	contentLines := len(visibleLines)
	
	// Add search status if needed (Match X of Y)
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
			contentLines++
		}
	}
	
	// Add search input if active
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
		contentLines += 3 // Search box takes 3 lines with border
	}
	
	// Calculate how much padding we need to push status bar to bottom
	// We need: contentLines + padding + 2 blank lines + status bar = m.height lines total
	// Which means: contentLines + padding + 2 + 1 = m.height
	// So: padding = m.height - contentLines - 3
	// BUT: we need one extra line because status bar shares the last line
	linesNeededForStatusBar := 2 + m.toastGutterRows() // 2 blank lines above, plus any reserved gutter below
	availableLinesForPadding := m.height - contentLines - linesNeededForStatusBar // status bar doesn't need a separate line count
	
	// Add padding to push status bar to bottom
	if availableLinesForPadding > 0 {
		for i := 0; i < availableLinesForPadding; i++ {
			result += "\n"
		}
	}
	
	// Add 2 blank lines above the status bar (margin top)
	result += "\n\n"
	
	// Add status bar, then any reserved gutter row beneath it
	result += m.renderStatusBar()
	result += strings.Repeat("\n", m.toastGutterRows())

	return result
}

// visibleWidth returns the display width of s in terminal columns, ignoring
// escape sequences. Wide characters count as 2 and combining marks as 0, so
// this matches the column numbers the terminal reports for mouse events.
func visibleWidth(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

// escapeEnd returns the index just past the escape sequence starting at i,
// along with whether it was a recognized CSI or OSC sequence. An unrecognized
// escape advances a single byte.
func escapeEnd(b []byte, i int) (int, bool) {
	start := i
	i++ // consume ESC
	if i < len(b) && b[i] == '[' {
		// CSI escape (\x1b[...m)
		i++
		for i < len(b) && !((b[i] >= 'A' && b[i] <= 'Z') || (b[i] >= 'a' && b[i] <= 'z')) {
			i++
		}
		if i < len(b) {
			i++
		}
		return i, true
	}
	if i < len(b) && b[i] == ']' {
		// OSC escape (\x1b]...\x1b\\)
		i++
		for i < len(b)-1 {
			if b[i] == '\x1b' && i+1 < len(b) && b[i+1] == '\\' {
				i += 2
				break
			}
			i++
		}
		return i, true
	}
	return start + 1, false
}

// truncateVisibleChars returns the prefix of s occupying at most n display
// columns. A wide character that would straddle the cut is dropped rather than
// split, so the result may be one column short of n.
func truncateVisibleChars(s string, n int) string {
	var result strings.Builder
	col := 0
	i := 0
	b := []byte(s)

	for i < len(b) {
		if b[i] == '\x1b' {
			escStart := i
			i, _ = escapeEnd(b, i)
			result.Write(b[escStart:i])
			continue
		}

		r, size := decodeRuneInBytes(b[i:])
		w := runewidth.RuneWidth(r)
		if col+w > n {
			break
		}
		result.WriteRune(r)
		i += size
		col += w
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

// skipVisibleChars returns the suffix of s starting at display column n,
// carrying forward the escape sequences that were active at the cut. A wide
// character straddling the cut cannot be split, so its orphaned cells are
// emitted as spaces to keep the suffix exactly (width - n) columns wide.
//
// Every recognized sequence is carried forward, including those before the
// cut: escapes occupy no columns, and dropping them would strand the suffix
// with the wrong colors and break an OSC 8 link that the cut lands inside.
func skipVisibleChars(s string, n int) string {
	col := 0
	i := 0
	b := []byte(s)
	var pendingEscapes strings.Builder

	for i < len(b) {
		if b[i] == '\x1b' {
			escStart := i
			end, recognized := escapeEnd(b, i)
			i = end
			if recognized {
				pendingEscapes.Write(b[escStart:end])
			}
			continue
		}

		if col >= n {
			break
		}

		r, size := decodeRuneInBytes(b[i:])
		i += size
		col += runewidth.RuneWidth(r)
	}

	// Nothing of the line reaches column n, so there is nothing to style.
	// Returning the escapes alone would leak their state into the next line.
	if i >= len(b) && col <= n {
		return ""
	}

	var result strings.Builder
	result.WriteString(pendingEscapes.String())
	if col > n {
		result.WriteString(strings.Repeat(" ", col-n))
	}
	result.Write(b[i:])
	return result.String()
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

	// If we found matches, scroll to the first one
	if match, ok := m.search.GetCurrentMatch(); ok {
		m = m.scrollToLine(match.lineNumber)
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
	
	if match, ok := m.search.NextMatch(); ok {
		m = m.scrollToLine(match.lineNumber)
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
	visibleHeight -= 2 // Reserve 2 blank lines above status bar
	visibleHeight -= m.toastGutterRows()
	if m.searchActive {
		visibleHeight -= 3 // Reserve space for search input
	}
	if m.search.term != "" {
		visibleHeight -= 1 // Reserve space for search status (Match X of Y)
	}
	// Note: We don't subtract 1 for the status bar itself because the status bar 
	// shares a line with the last newline from the content
	
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
