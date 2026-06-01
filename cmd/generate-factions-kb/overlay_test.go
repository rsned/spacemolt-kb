package main

import "testing"

func TestSplitFrontmatter(t *testing.T) {
	withFM := "---\nimage: logo.png\n---\n## Body\n\nHello\n"
	front, body := splitFrontmatter([]byte(withFM))
	if string(front) != "image: logo.png" {
		t.Errorf("front = %q", front)
	}
	if body != "## Body\n\nHello\n" {
		t.Errorf("body = %q", body)
	}

	// No frontmatter: everything is body.
	front, body = splitFrontmatter([]byte("just a body\n"))
	if front != nil {
		t.Errorf("expected nil front, got %q", front)
	}
	if body != "just a body\n" {
		t.Errorf("body = %q", body)
	}

	// Unterminated frontmatter: treat whole thing as body, no front.
	front, body = splitFrontmatter([]byte("---\nimage: x\nno closing fence\n"))
	if front != nil {
		t.Errorf("expected nil front for unterminated, got %q", front)
	}

	// CRLF normalized.
	front, _ = splitFrontmatter([]byte("---\r\nimage: a.png\r\n---\r\nbody"))
	if string(front) != "image: a.png" {
		t.Errorf("CRLF front = %q", front)
	}
}
