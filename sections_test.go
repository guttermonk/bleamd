package main

import (
	"strings"
	"testing"

	"github.com/MichaelMure/go-term-markdown"
)

// renderSource mirrors what model.render does, minus the terminal dependent
// parts, and returns the output without its escape sequences.
func renderSource(t *testing.T, source string, keepNumbers bool) string {
	t.Helper()

	out := markdown.Render(source, 80, padding)
	out = cleanHeadings(out, source, keepNumbers)

	return stripAllEscapeSequences(string(out))
}

func TestSourceHeadings(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   []heading
	}{
		{
			name:   "nested headings",
			source: "# One\n\n## Two\n\n### Three\n\n## Four\n\n# Five\n",
			want: []heading{
				{level: 1, number: "1", text: "One"},
				{level: 2, number: "1.1", text: "Two"},
				{level: 3, number: "1.1.1", text: "Three"},
				{level: 2, number: "1.2", text: "Four"},
				{level: 1, number: "2", text: "Five"},
			},
		},
		{
			name:   "skipped level",
			source: "# One\n\n### Deep\n",
			want: []heading{
				{level: 1, number: "1", text: "One"},
				{level: 3, number: "1.0.1", text: "Deep"},
			},
		},
		{
			name:   "setext headings",
			source: "One\n===\n\nTwo\n---\n",
			want: []heading{
				{level: 1, number: "1", text: "One"},
				{level: 2, number: "1.1", text: "Two"},
			},
		},
		{
			name:   "heading in a blockquote",
			source: "# One\n\n> ## Quoted\n",
			want: []heading{
				{level: 1, number: "1", text: "One"},
				{level: 2, number: "1.1", text: "Quoted"},
			},
		},
		{
			name:   "html heading",
			source: "# One\n\n<h2>Html</h2>\n\n## Two\n",
			want: []heading{
				{level: 1, number: "1", text: "One"},
				{level: 2, number: "1.1", text: "Html"},
				{level: 2, number: "1.2", text: "Two"},
			},
		},
		{
			name:   "styled heading",
			source: "## A `code` and a [link](https://example.com)\n",
			want: []heading{
				{level: 2, number: "0.1", text: "A code and a link"},
			},
		},
		{
			name:   "hashes inside a code block are not headings",
			source: "# One\n\n```\n# not a heading\n```\n\n## Two\n",
			want: []heading{
				{level: 1, number: "1", text: "One"},
				{level: 2, number: "1.1", text: "Two"},
			},
		},
		{
			name:   "no headings",
			source: "Just a paragraph.\n",
			want:   nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sourceHeadings(tc.source)
			if len(got) != len(tc.want) {
				t.Fatalf("sourceHeadings() = %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("sourceHeadings() = %+v, want %+v", got, tc.want)
				}
			}
		})
	}
}

func TestCleanHeadingsStripsNumbering(t *testing.T) {
	source := "# One\n\n## Two\n\n### Three\n\n## Four\n\n# Five\n"

	got := renderSource(t, source, false)

	for _, heading := range []string{"One", "Two", "Three", "Four", "Five"} {
		if !strings.Contains(got, heading) {
			t.Errorf("heading %q is missing from the output:\n%s", heading, got)
		}
	}
	for _, number := range []string{"1 One", "1.1 Two", "1.1.1 Three", "1.2 Four", "2 Five"} {
		if strings.Contains(got, number) {
			t.Errorf("section number %q was not stripped:\n%s", number, got)
		}
	}
}

func TestCleanHeadingsKeepsNumberingWithFlag(t *testing.T) {
	source := "# One\n\n## Two\n"

	got := renderSource(t, source, true)

	for _, number := range []string{"1 One", "1.1 Two"} {
		if !strings.Contains(got, number) {
			t.Errorf("section number %q is missing from the output:\n%s", number, got)
		}
	}
}

func TestCleanHeadingsStripsNumberingOfSpecialHeadings(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		rendered string
	}{
		{name: "quoted", source: "# One\n\n> ## Quoted\n", rendered: "Quoted"},
		{name: "html", source: "# One\n\n<h2>Html</h2>\n", rendered: "Html"},
		{name: "setext", source: "One\n===\n\nTwo\n---\n", rendered: "Two"},
		{name: "styled", source: "## A `code` and a [link](https://example.com)\n", rendered: "A code and a ["},
		{name: "emoji", source: "## :rocket: Launch\n", rendered: "Launch"},
		{name: "wrapped", source: "## " + strings.Repeat("word ", 40) + "\n", rendered: "word"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderSource(t, tc.source, false)

			if !strings.Contains(got, tc.rendered) {
				t.Fatalf("heading is missing from the output:\n%s", got)
			}
			for _, number := range sourceHeadings(tc.source) {
				if strings.Contains(got, number.number+" ") {
					t.Errorf("section number %q was not stripped:\n%s", number.number, got)
				}
			}
		})
	}
}

func TestCleanHeadingsLeavesContentAlone(t *testing.T) {
	// Every one of these starts with something that looks like a section
	// number, including the exact numbers the renderer emits for the headings.
	source := "# One\n\n2 is the number of headings here.\n\n```\n1.1 fake heading\n```\n\n1. ordered item\n\n# Two\n"

	got := renderSource(t, source, false)

	for _, content := range []string{"2 is the number of headings here.", "1.1 fake heading", "1. ordered item"} {
		if !strings.Contains(got, content) {
			t.Errorf("content %q was altered:\n%s", content, got)
		}
	}
	if strings.Contains(got, "1 One") || strings.Contains(got, "2 Two") {
		t.Errorf("headings were not stripped:\n%s", got)
	}
}

func TestCleanHeadingsRemovesHeadingUnderline(t *testing.T) {
	got := renderSource(t, "# One\n\nSome text.\n\n## Two\n", false)

	if strings.Contains(got, "───") {
		t.Errorf("the heading underline was not removed:\n%s", got)
	}
}

func TestCleanHeadingsKeepsThematicBreaks(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{name: "after a heading", source: "# One\n\n---\n\nSome text.\n"},
		{name: "between paragraphs", source: "Some text.\n\n***\n\nMore text.\n"},
		{name: "first in the document", source: "___\n\nSome text.\n"},
		{name: "after a numbered heading", source: "# One\n\n---\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderSource(t, tc.source, false)

			if !strings.Contains(got, "───") {
				t.Errorf("the thematic break was removed:\n%s", got)
			}
		})
	}
}

func TestCleanHeadingsPullsThematicBreakUpToHeading(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		heading string
	}{
		{name: "after h1", source: "# One\n\n---\n\nSome text.\n", heading: "One"},
		{name: "after h2", source: "## Two\n\n---\n\nSome text.\n", heading: "Two"},
		{name: "with a link", source: "# [One](https://example.com)\n\n---\n\nSome text.\n", heading: "One"},
		{name: "numbered", source: "# One\n\n---\n\nSome text.\n", heading: "One"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderSource(t, tc.source, tc.name == "numbered")

			lines := strings.Split(got, "\n")
			at := -1
			for i, line := range lines {
				if strings.Contains(line, tc.heading) {
					at = i
					break
				}
			}
			if at < 0 {
				t.Fatalf("heading is missing from the output:\n%s", got)
			}
			if at+1 >= len(lines) || !strings.Contains(lines[at+1], "─") {
				t.Errorf("the thematic break should sit right below the heading:\n%s", got)
			}
		})
	}
}

func TestCleanHeadingsKeepsThematicBreakSpacingAfterText(t *testing.T) {
	got := renderSource(t, "# One\n\nSome text.\n\n---\n\nMore text.\n", false)

	lines := strings.Split(got, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "─") {
			continue
		}
		if i == 0 || strings.TrimSpace(lines[i-1]) != "" {
			t.Errorf("a thematic break away from a heading keeps its empty line:\n%s", got)
		}
		return
	}

	t.Fatalf("the thematic break is missing from the output:\n%s", got)
}

func TestCleanHeadingsKeepsTables(t *testing.T) {
	source := "# One\n\n| a | b |\n| - | - |\n| 1 | 2 |\n"

	got := renderSource(t, source, false)

	for _, border := range []string{"┌", "╞", "└"} {
		if !strings.Contains(got, border) {
			t.Errorf("the table border %q was removed:\n%s", border, got)
		}
	}
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name        string
		argv        []string
		wantArgs    []string
		wantNumbers bool
	}{
		{
			name:        "no flags",
			argv:        []string{"README.md"},
			wantArgs:    []string{"README.md"},
			wantNumbers: false,
		},
		{
			name:        "long flag before the file",
			argv:        []string{"--section-numbers", "README.md"},
			wantArgs:    []string{"README.md"},
			wantNumbers: true,
		},
		{
			name:        "short flag after the file",
			argv:        []string{"README.md", "-sn"},
			wantArgs:    []string{"README.md"},
			wantNumbers: true,
		},
		{
			name:        "other flags are passed through",
			argv:        []string{"--init-config", "dracula"},
			wantArgs:    []string{"--init-config", "dracula"},
			wantNumbers: false,
		},
		{
			name:        "stdin",
			argv:        []string{"-sn"},
			wantArgs:    nil,
			wantNumbers: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args, numbers := parseArgs(tc.argv)
			if numbers != tc.wantNumbers {
				t.Errorf("parseArgs(%v) sectionNumbers = %v, want %v", tc.argv, numbers, tc.wantNumbers)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("parseArgs(%v) args = %v, want %v", tc.argv, args, tc.wantArgs)
			}
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Fatalf("parseArgs(%v) args = %v, want %v", tc.argv, args, tc.wantArgs)
				}
			}
		})
	}
}
