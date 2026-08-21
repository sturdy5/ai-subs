package main

import (
	"testing"
	"os"
)

func TestSplitTextIntoChunks(t *testing.T) {
	text := "This is a sample text that will be split into chunks for testing purposes."
	numChunks := 3
	chunks := splitTextIntoChunks(text, numChunks)

	if len(chunks) != numChunks {
		t.Errorf("Expected %d chunks, but got %d", numChunks, len(chunks))
	}

	for i, chunk := range chunks {
		t.Logf("Chunk %d: %s", i+1, chunk)
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"12345", true},
		{"abc123", false},
		{"", false},
		{"0000", true},
	}

	for _, test := range tests {
		result := isNumeric(test.input)
		if result != test.expected {
			t.Errorf("isNumeric(%q) = %v; want %v", test.input, result, test.expected)
		}
	}
}

func TestParseSRT(t *testing.T) {
	srtContent := `1
00:00:01,000 --> 00:00:04,000
Hello, this is the first subtitle.

2
00:00:05,000 --> 00:00:08,000
And this is the second subtitle.
`
	// Create a temporary SRT file for testing
	tmpFile, err := os.CreateTemp("", "test.srt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(srtContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	dialogue, err := parseSRT(tmpFile.Name())
	if err != nil {
		t.Fatalf("parseSRT failed: %v", err)
	}

	expectedDialogue := "Hello, this is the first subtitle. And this is the second subtitle."
	if dialogue != expectedDialogue {
		t.Errorf("Expected dialogue %q, but got %q", expectedDialogue, dialogue)
	}
}

func TestCopyFile(t *testing.T) {
	srcContent := "This is a test file."
	// Create a temporary source file for testing
	srcFile, err := os.CreateTemp("", "src.txt")
	if err != nil {
		t.Fatalf("Failed to create temp source file: %v", err)
	}
	defer os.Remove(srcFile.Name())

	if _, err := srcFile.WriteString(srcContent); err != nil {
		t.Fatalf("Failed to write to temp source file: %v", err)
	}
	srcFile.Close()

	// Create a temporary destination file path
	dstFile, err := os.CreateTemp("", "dst.txt")
	if err != nil {
		t.Fatalf("Failed to create temp destination file: %v", err)
	}
	defer os.Remove(dstFile.Name())
	dstFile.Close()

	if err := copyFile(srcFile.Name(), dstFile.Name()); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	// Read the content of the destination file
	dstContent, err := os.ReadFile(dstFile.Name())
	if err != nil {
		t.Fatalf("Failed to read temp destination file: %v", err)
	}

	if string(dstContent) != srcContent {
		t.Errorf("Expected destination content %q, but got %q", srcContent, string(dstContent))
	}
}
