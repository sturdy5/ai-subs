package main

import (
	"bytes"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type OllamaOptions struct {
	NumPredict int     `json:"num_predict"`
	Temperature float64 `json:"temperature"`
}

type OllamaRequest struct {
	Model   string        `json:"model"`
	Prompt  string        `json:"prompt"`
	Stream  bool          `json:"stream"`
	Options OllamaOptions `json:"options"`
}

type OllamaResponse struct {
	Response string `json:"response"`
}

// TMDBResponse matches the basic structure returned by the TMDB Movie Details API
type TMDBResponse struct {
	Title string `json:"title"`
}

func main() {
	// Get the ollma URL and TMDB token from an environment variable
	ollamaURL := os.ExpandEnv("$OLLAMA_URL")
	tmdbToken := os.ExpandEnv("$TMDB_TOKEN")
	moviesDir := os.ExpandEnv("$MEDIA_DIR")

	// check to make sure the environment variables are set
	if ollamaURL == "" || tmdbToken == "" || moviesDir == "" {
		fmt.Println("Error: One or more required environment variables are not set.")
		fmt.Println("Please ensure OLLAMA_URL, TMDB_TOKEN, and MEDIA_DIR are set.")
		return
	}

	fmt.Printf("Starting media scan and NFO updates in: %s\n\n", moviesDir)

	err := filepath.WalkDir(moviesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("Error accessing path %s: %v\n", path, err)
			return nil
		}

		if !d.IsDir() && strings.ToLower(filepath.Ext(path)) == ".srt" {
			nfoPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".nfo"

			if isAlreadyProcessed(nfoPath) {
				fmt.Printf("Skipping already processed movie: %s\n", d.Name())
				return nil
			}

			fmt.Printf("========================================\n")
			fmt.Printf("Processing: %s\n", d.Name())
			processMovieSubtitle(ollamaURL, tmdbToken, path)
			fmt.Printf("========================================\n\n")
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking the path: %v\n", err)
	}
}

func isAlreadyProcessed(nfoPath string) bool {
	if _, err := os.Stat(nfoPath); os.IsNotExist(err) {
		return false // No file means it definitely isn't processed
	}

	content, err := os.ReadFile(nfoPath)
	if err != nil {
		return false
	}

	// Look for our specific custom indicator string
	return strings.Contains(string(content), "<aisummary>processed</aisummary>")
}

func processMovieSubtitle(ollamaURL, tmdbToken, srtPath string) {
	nfoPath := strings.TrimSuffix(srtPath, filepath.Ext(srtPath)) + ".nfo"

	cleanDialogue, err := parseSRT(srtPath)
	if err != nil || len(cleanDialogue) == 0 {
		fmt.Printf("Skipping file (could not parse or empty): %v\n", err)
		return
	}

	chunks := splitTextIntoChunks(cleanDialogue, 4)
	var chunkSummaries []string

	for i, chunk := range chunks {
		prompt := fmt.Sprintf(
			"[INST] Summarize the key plot developments in this section of the movie script in 2 short sentences. Do not use meta-commentary. [/INST]\n\nDialogue: %s", 
			chunk,
		)

		summary, err := callOllama(ollamaURL, prompt, 100)
		if err != nil {
			fmt.Printf("   Error processing chunk %d: %v\n", i+1, err)
			return
		}
		chunkSummaries = append(chunkSummaries, summary)
	}

	combinedSummaries := strings.Join(chunkSummaries, " ")

	crankyPrompt := fmt.Sprintf(
		"[INST] You are a grumpy, cynical, cranky old man who hates modern cinema and loves to roast movies. "+
		"Review these chronological plot points of a movie. Write exactly ONE short paragraph (3-4 sentences max) "+
		"summarizing the premise while ruthlessly roasting the characters, clichés, or plot setup. "+
		"Talk about it like you are complaining to your neighbors. "+
		"STRICT RULES: "+
		"1. Do NOT reveal major twists, endings, or spoilers. "+
		"2. Stay strictly in character as an old man. "+
		"3. Do NOT mention that this is a script. "+
		"4. Keep it to one single paragraph. No bullet points, no bold fonts. [/INST]\n\nPlot Points: %s",
		combinedSummaries,
	)

	startTime := time.Now()
	finalResponse, err := callOllama(ollamaURL, crankyPrompt, 200)
	if err != nil {
		fmt.Printf("Error generating roasted summary: %v\n", err)
		return
	}

	fmt.Printf("Generated Roast (%v):\n\"%s\"\n\n", time.Since(startTime), finalResponse)

	// Modified: Pass the TMDB token down to handle potential generation lookups
	err = backupAndUpdateNFO(nfoPath, tmdbToken, finalResponse)
	if err != nil {
		fmt.Printf("Failed to update NFO file: %v\n", err)
	}
}

func backupAndUpdateNFO(nfoPath, tmdbToken, newPlot string) error {
	backupPath := nfoPath + ".bak"

	// If NFO doesn't exist, use our new TMDB lookup function to build it cleanly
	if _, err := os.Stat(nfoPath); os.IsNotExist(err) {
		fmt.Println("No existing NFO found. Pulling official title from TMDB...")
		return createMinimalNFO(nfoPath, tmdbToken, newPlot)
	}

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		fmt.Println("Creating original NFO backup at:", filepath.Base(backupPath))
		err := copyFile(nfoPath, backupPath)
		if err != nil {
			return fmt.Errorf("failed to create backup file: %w", err)
		}
	}

	input, err := os.ReadFile(nfoPath)
	if err != nil {
		return err
	}

	content := string(input)
	plotRegex := regexp.MustCompile(`(?s)<plot>.*?</plot>`)
	updatedPlotTag := fmt.Sprintf("<plot>%s</plot>\n  <aisummary>processed</aisummary>", newPlot)

	var finalContent string
	if plotRegex.MatchString(content) {
		finalContent = plotRegex.ReplaceAllString(content, updatedPlotTag)
	} else {
		finalContent = strings.Replace(content, "</movie>", fmt.Sprintf("  %s\n</movie>", updatedPlotTag), 1)
	}

	err = os.WriteFile(nfoPath, []byte(finalContent), 0644)
	if err != nil {
		return err
	}

	fmt.Println("Successfully updated NFO file with AI review.")
	return nil
}

func createMinimalNFO(nfoPath, tmdbToken, newPlot string) error {
	folderName := filepath.Base(filepath.Dir(nfoPath))
	idRegex := regexp.MustCompile(`\[tmdbid-(\d+)\]`)
	matches := idRegex.FindStringSubmatch(folderName)

	movieTitle := folderName
	if len(matches) > 1 {
		tmdbID := matches[1]
		fmt.Printf("   Found TMDB ID: %s. Fetching official title...\n", tmdbID)
		
		fetchedTitle, err := fetchTitleFromTMDB(tmdbID, tmdbToken)
		if err == nil && fetchedTitle != "" {
			movieTitle = fetchedTitle
			fmt.Printf("   TMDB Resolved Title: %s\n", movieTitle)
		}
	}
	
	template := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<movie>
  <title>%s</title>
  <plot>%s</plot>
  <aisummary>processed</aisummary>
</movie>`, movieTitle, newPlot)

	return os.WriteFile(nfoPath, []byte(template), 0644)
}

// New Helper: Makes a secure HTTP request to TMDB API v4 using your Bearer token
func fetchTitleFromTMDB(tmdbID, token string) (string, error) {
	url := fmt.Sprintf("https://api.themoviedb.org/3/movie/%s", tmdbID)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// TMDB authentication requires providing the Read Access Token as a Bearer header
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api responded with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var tmdbResp TMDBResponse
	err = json.Unmarshal(body, &tmdbResp)
	if err != nil {
		return "", err
	}

	return tmdbResp.Title, nil
}

func callOllama(url, prompt string, maxTokens int) (string, error) {
	requestData := OllamaRequest{
		Model:  "llama3.2:1b",
		Prompt: prompt,
		Stream: false,
		Options: OllamaOptions{
			NumPredict:  maxTokens,
			Temperature: 0.7,
		},
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var ollamaResp OllamaResponse
	err = json.Unmarshal(body, &ollamaResp)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(ollamaResp.Response), nil
}

func splitTextIntoChunks(text string, numChunks int) []string {
	words := strings.Fields(text)
	totalWords := len(words)
	chunkSize := totalWords / numChunks

	var chunks []string
	for i := 0; i < numChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if i == numChunks-1 {
			end = totalWords
		}
		chunkWords := words[start:end]
		chunks = append(chunks, strings.Join(chunkWords, " "))
	}
	return chunks
}

func parseSRT(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	timestampRegex := regexp.MustCompile(`\d{2}:\d{2}:\d{2}`)
	var dialogueLines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || isNumeric(line) || timestampRegex.MatchString(line) {
			continue
		}
		dialogueLines = append(dialogueLines, line)
	}

	return strings.Join(dialogueLines, " "), scanner.Err()
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
