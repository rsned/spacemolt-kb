package main

import (
	"os"
	"strings"
	"testing"
)

func TestViewerAssetsPresent(t *testing.T) {
	idx, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`src="app.js"`, `id="slider"`, `id="sphere"`, `id="cross"`, `id="bundle"`} {
		if !strings.Contains(string(idx), want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	js, err := os.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/api/bundles", "/api/run", "/api/jobs/", "manifest.json", "gl_FragColor", "NEAREST"} {
		if !strings.Contains(string(js), want) {
			t.Errorf("app.js lacks %s", want)
		}
	}
}
