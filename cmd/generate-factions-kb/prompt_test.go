package main

import (
	"strings"
	"testing"
)

func TestBuildPortraitPrompt(t *testing.T) {
	p := buildPortraitPrompt("a grizzled ore hauler", "first", "crimson")
	if !strings.Contains(p, "a grizzled ore hauler") {
		t.Fatal("bio missing from prompt")
	}
	if !strings.Contains(p, "refined affluent attire") {
		t.Fatal("first-class attire cue missing")
	}
	if !strings.Contains(p, "crimson empire") {
		t.Fatal("citizenship theme missing")
	}
	if !strings.Contains(p, portraitStyleSuffix) {
		t.Fatal("style suffix missing")
	}
}

func TestBuildPortraitPromptEmptyBioIsValid(t *testing.T) {
	p := buildPortraitPrompt("", "", "")
	if strings.TrimSpace(p) == "" {
		t.Fatal("empty bio produced empty prompt")
	}
	if !strings.Contains(p, portraitStyleSuffix) {
		t.Fatal("style suffix missing on fallback prompt")
	}
}

func TestPromptHashStable(t *testing.T) {
	a := promptHash("hello")
	if a != promptHash("hello") {
		t.Fatal("promptHash not stable")
	}
	if a == promptHash("hellp") {
		t.Fatal("promptHash collided")
	}
}
