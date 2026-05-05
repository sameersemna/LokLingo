package jobs

import "testing"

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "collapses spaces within a line",
			input: "hello   world  foo",
			want:  "hello world foo",
		},
		{
			name:  "collapses tabs and mixed whitespace",
			input: "hello\t\tworld\t foo",
			want:  "hello world foo",
		},
		{
			name:  "preserves paragraph break",
			input: "paragraph one\n\nparagraph two",
			want:  "paragraph one\n\nparagraph two",
		},
		{
			name:  "normalizes whitespace within each paragraph",
			input: "first  line  here\n\nsecond   line  there",
			want:  "first line here\n\nsecond line there",
		},
		{
			name:  "collapses newlines within a paragraph into a space",
			input: "line one\nline two\n\nline three",
			want:  "line one line two\n\nline three",
		},
		{
			name:  "trims leading and trailing whitespace",
			input: "  \n\nhello world\n\n  ",
			want:  "hello world",
		},
		{
			name:  "removes blank paragraph entries",
			input: "para one\n\n\n\npara two",
			want:  "para one\n\npara two",
		},
		{
			name:  "empty string returns empty string",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace-only string returns empty string",
			input: "   \t\n\n  ",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeText(tc.input)
			if got != tc.want {
				t.Fatalf("normalizeText(%q)\n  got:  %q\n  want: %q", tc.input, got, tc.want)
			}
		})
	}
}
