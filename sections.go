package main

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/MichaelMure/go-term-markdown"
	md "github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
)

// go-term-markdown always prefixes headings with a section number ("1",
// "1.2", ...) and always underlines level 1 headings with a horizontal rule.
// Neither can be turned off through the renderer API, so we undo them on the
// rendered output: numbering is opt-in in bleamd (--section-numbers) and a
// horizontal rule should only show up where the document actually has a
// thematic break.
//
// To strip the numbers safely we replay the renderer's numbering over the
// markdown source, which tells us exactly which numbers it emitted, in which
// order, and for which heading. A rendered line is only touched when it
// carries the number we are looking for and the heading text that goes with
// it, so prose and code that happens to start with a number is left alone.

// renderedHeadingNumber matches a rendered heading: the left padding (plus any
// blockquote bars the heading sits behind), the color escapes of the heading
// shade, and then the section number itself.
var renderedHeadingNumber = regexp.MustCompile(`^(?:\x1b\[[0-9;]*m|[ \t┃])*(\d+(?:\.\d+)*) `)

// renderedRule matches a line holding nothing but a horizontal rule.
var renderedRule = regexp.MustCompile(`^(?:\x1b\[[0-9;]*m|[ \t┃])*─+(?:\x1b\[[0-9;]*m)*$`)

// htmlHeading matches the HTML headings that the renderer numbers too.
var htmlHeading = regexp.MustCompile(`(?is)<h([1-6])[^>]*>(.*?)</h[1-6]>`)

// htmlTag matches any HTML tag, used to get at the text of an HTML heading.
var htmlTag = regexp.MustCompile(`(?s)<[^>]*>`)

// insignificant matches everything that is dropped before comparing a heading
// of the source with its rendered counterpart. The renderer rewrites headings
// along the way (emoji shortcodes, links, inline styling), so only letters and
// digits are reliably comparable.
var insignificant = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// heading is a heading of the source document, as the renderer numbers it.
type heading struct {
	level  int
	number string
	text   string
}

// headingNumbering mirrors the numbering state machine of go-term-markdown.
type headingNumbering struct {
	levels [6]int
}

// observe registers a heading of the given level and returns the resulting
// numbering state.
func (hn headingNumbering) observe(level int) headingNumbering {
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}

	hn.levels[level-1]++
	for i := level; i < 6; i++ {
		hn.levels[i] = 0
	}

	return hn
}

// String renders the current numbering, dropping the trailing unused levels.
func (hn headingNumbering) String() string {
	last := -1
	for i := 5; i >= 0; i-- {
		if hn.levels[i] != 0 {
			last = i
			break
		}
	}

	parts := make([]string, 0, last+1)
	for i := 0; i <= last; i++ {
		parts = append(parts, strconv.Itoa(hn.levels[i]))
	}

	return strings.Join(parts, ".")
}

// sourceHeadings returns the headings of source in document order, each with
// the section number go-term-markdown gives it.
func sourceHeadings(source string) []heading {
	doc := md.Parse([]byte(source), parser.NewWithExtensions(markdown.Extensions()))

	var (
		numbering headingNumbering
		headings  []heading
	)

	add := func(level int, text string) {
		numbering = numbering.observe(level)
		headings = append(headings, heading{
			level:  level,
			number: numbering.String(),
			text:   text,
		})
	}

	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}

		switch node := node.(type) {
		case *ast.Heading:
			add(node.Level, nodeText(node))

		case *ast.HTMLBlock:
			// The renderer walks the HTML of a block and renders <h1> to <h6>
			// as headings, numbering included.
			for _, tag := range htmlHeading.FindAllSubmatch(node.Literal, -1) {
				level, err := strconv.Atoi(string(tag[1]))
				if err != nil {
					continue
				}
				add(level, htmlTag.ReplaceAllString(string(tag[2]), ""))
			}
		}

		return ast.GoToNext
	})

	return headings
}

// nodeText collects the text of a node and of everything below it.
func nodeText(node ast.Node) string {
	var text strings.Builder

	ast.WalkFunc(node, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}
		switch node := node.(type) {
		case *ast.Text:
			text.Write(node.Literal)
		case *ast.Code:
			text.Write(node.Literal)
		}
		return ast.GoToNext
	})

	return text.String()
}

// comparable reduces a heading to the characters that survive rendering.
func comparable(text string) string {
	return strings.ToLower(insignificant.ReplaceAllString(text, ""))
}

// cleanHeadings undoes the heading decorations go-term-markdown adds on its
// own: the section numbers, unless keepNumbers says the user asked for them,
// and the horizontal rule under every level 1 heading.
func cleanHeadings(rendered []byte, source string, keepNumbers bool) []byte {
	lines := bytes.Split(rendered, []byte("\n"))
	headings := sourceHeadings(source)

	dropped := make(map[int]bool)

	next := 0
	for i, line := range lines {
		if next >= len(headings) {
			break
		}

		at := matchHeading(line, headings[next:])
		if at < 0 {
			continue
		}
		at += next
		next = at + 1

		if !keepNumbers {
			loc := renderedHeadingNumber.FindSubmatchIndex(line)
			stripped := make([]byte, 0, len(line)-(loc[1]-loc[2]))
			stripped = append(stripped, line[:loc[2]]...)
			stripped = append(stripped, line[loc[1]:]...)
			lines[i] = stripped
		}

		for _, drop := range rulesToDrop(lines, i) {
			dropped[drop] = true
		}
	}

	if len(dropped) == 0 {
		return bytes.Join(lines, []byte("\n"))
	}

	kept := make([][]byte, 0, len(lines)-len(dropped))
	for i, line := range lines {
		if !dropped[i] {
			kept = append(kept, line)
		}
	}

	return bytes.Join(kept, []byte("\n"))
}

// matchHeading reports which of the headings, if any, the rendered line holds.
// Headings are matched in order on both their section number and their text,
// so that a line of prose or code starting with a number is left alone.
func matchHeading(line []byte, headings []heading) int {
	loc := renderedHeadingNumber.FindSubmatchIndex(line)
	if loc == nil {
		return -1
	}

	number := string(line[loc[2]:loc[3]])
	text := comparable(stripAllEscapeSequences(string(line[loc[1]:])))

	for i, heading := range headings {
		if heading.number != number {
			continue
		}

		// The renderer wraps long headings and rewrites their text, so the two
		// sides only have to recognize each other, not be equal.
		want := comparable(heading.text)
		if want == "" || text == "" ||
			strings.Contains(want, text) || strings.Contains(text, want) {
			return i
		}

		return -1
	}

	return -1
}

// rulesToDrop returns the lines to remove below the heading starting at line
// i so that it carries a horizontal rule only when the document says so:
//
//   - the rule the renderer draws under every level 1 heading goes away, it
//     sits right below the heading with no empty line in between;
//   - a thematic break of the document, which the renderer separates from the
//     heading by an empty line, stays and moves up against the heading by
//     dropping that empty line.
func rulesToDrop(lines [][]byte, i int) []int {
	var drop []int

	// Walk over the heading itself, which the renderer may have wrapped over
	// several lines, down to the empty line that closes it.
	j := i + 1
	for ; j < len(lines) && !blankLine(lines[j]); j++ {
		if renderedRule.Match(lines[j]) {
			drop = append(drop, j)
			j++
			break
		}
	}

	if j < len(lines) && blankLine(lines[j]) &&
		j+1 < len(lines) && renderedRule.Match(lines[j+1]) {
		drop = append(drop, j)
	}

	return drop
}

// blankLine reports whether a rendered line carries no visible content.
func blankLine(line []byte) bool {
	visible := strings.Trim(stripAllEscapeSequences(string(line)), " \t┃")
	return visible == ""
}
