package watcher

import (
	"testing"
)

func TestIsGoFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"pkg/handler.go", true},
		{"/home/user/project/cmd/main.go", true},
		{"main.py", false},
		{"go.mod", false},
		{"README.md", false},
		{"file.go.bak", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isGoFile(tt.path); got != tt.want {
			t.Errorf("isGoFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
