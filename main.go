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

var aiModel = os.ExpandEnv("$AI_MODEL")

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

		// Only process .srt files
		if !d.IsDir() && strings.ToLower(filepath.Ext(path)) == ".srt" {
			// Construct the corresponding NFO path
			nfoPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".nfo"

			// Check if the NFO file already contains our custom indicator
			if isAlreadyProcessed(nfoPath) {
				fmt.Printf("Skipping already processed movie: %s\n", d.Name())
				return nil
			}

			fmt.Printf("========================================\n")
			fmt.Printf("Processing: %s\n", d.Name())
			processMovieSubtitle(ollamaURL, tmdbToken, path, nfoPath)
			fmt.Printf("========================================\n\n")
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking the path: %v\n", err)
	}
}

// Checks if the NFO file already contains our custom indicator to avoid reprocessing
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

// Processes a single movie subtitle file, generates a roasted summary, and updates the NFO file
func processMovieSubtitle(ollamaURL, tmdbToken, srtPath, nfoPath string) {
	cleanDialogue, err := parseSRT(srtPath)
	if err != nil || len(cleanDialogue) == 0 {
		fmt.Printf("Skipping file (could not parse or empty): %v\n", err)
		return
	}

	// Split the dialogue into 4 chunks for summarization
	chunks := splitTextIntoChunks(cleanDialogue, 4)
	var chunkSummaries []string

	// For each chunk, generate a concise summary using the Ollama API
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
		// Append the summary for this chunk to the list of summaries
		chunkSummaries = append(chunkSummaries, summary)
	}

	// Combine all chunk summaries into a single string for the final roasting prompt
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

	// Measure the time taken to generate the final roasted summary
	startTime := time.Now()
	finalResponse, err := callOllama(ollamaURL, crankyPrompt, 200)
	if err != nil {
		fmt.Printf("Error generating roasted summary: %v\n", err)
		return
	}

	fmt.Printf("Generated Roast (%v):\n\"%s\"\n\n", time.Since(startTime), finalResponse)

	// Backup the original NFO and update it with the new roasted summary
	err = backupAndUpdateNFO(nfoPath, tmdbToken, finalResponse)
	if err != nil {
		fmt.Printf("Failed to update NFO file: %v\n", err)
	}
}

// Backs up the original NFO file and updates it with the new plot summary
func backupAndUpdateNFO(nfoPath, tmdbToken, newPlot string) error {
	backupPath := nfoPath + ".bak"

	// If NFO doesn't exist, use our new TMDB lookup function to build it cleanly
	if _, err := os.Stat(nfoPath); os.IsNotExist(err) {
		fmt.Println("No existing NFO found. Pulling official title from TMDB...")
		return createMinimalNFO(nfoPath, tmdbToken, newPlot)
	}

	// Backup the original NFO if it doesn't already exist
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

	// Use regex to find the <plot> tag and replace it with the new plot summary, or append if not found
	content := string(input)
	plotRegex := regexp.MustCompile(`(?s)<plot>.*?</plot>`)
	// Construct the updated plot tag with our custom indicator
	updatedPlotTag := fmt.Sprintf("<plot>%s</plot>\n  <aisummary>processed</aisummary>", newPlot)

	var finalContent string
	if plotRegex.MatchString(content) {
		finalContent = plotRegex.ReplaceAllString(content, updatedPlotTag)
	} else {
		finalContent = strings.Replace(content, "</movie>", fmt.Sprintf("  %s\n</movie>", updatedPlotTag), 1)
	}

	// Write the updated content back to the NFO file
	err = os.WriteFile(nfoPath, []byte(finalContent), 0644)
	if err != nil {
		return err
	}

	fmt.Println("Successfully updated NFO file with AI review.")
	return nil
}

// Creates a minimal NFO file with the official title from TMDB and the new plot summary
func createMinimalNFO(nfoPath, tmdbToken, newPlot string) error {
	folderName := filepath.Base(filepath.Dir(nfoPath))
	// Use regex to extract the TMDB ID from the folder name, if present
	idRegex := regexp.MustCompile(`\[tmdbid-(\d+)\]`)
	matches := idRegex.FindStringSubmatch(folderName)

	// Default to using the folder name as the movie title if no TMDB ID is found
	movieTitle := folderName
	// If a TMDB ID is found, fetch the official title from TMDB
	if len(matches) > 1 {
		tmdbID := matches[1]
		fmt.Printf("   Found TMDB ID: %s. Fetching official title...\n", tmdbID)
		
		fetchedTitle, err := fetchTitleFromTMDB(tmdbID, tmdbToken)
		if err == nil && fetchedTitle != "" {
			// If the title is successfully fetched, use it as the movie title
			movieTitle = fetchedTitle
			fmt.Printf("   TMDB Resolved Title: %s\n", movieTitle)
		}
	}
	
	// Construct a minimal NFO template with the movie title and new plot summary
	template := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<movie>
  <title>%s</title>
  <plot>%s</plot>
  <aisummary>processed</aisummary>
</movie>`, movieTitle, newPlot)

	return os.WriteFile(nfoPath, []byte(template), 0644)
}

//  Makes a secure HTTP request to TMDB API v4 using your Bearer token
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

	// Return the official title fetched from TMDB
	return tmdbResp.Title, nil
}

// Calls the Ollama API with the provided prompt and returns the generated response
func callOllama(url, prompt string, maxTokens int) (string, error) {
	if aiModel == "" {
		aiModel = "llama3.2:1b" // Default model if not set in environment
	}
	requestData := OllamaRequest{
		Model:  aiModel,
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

	// Return the trimmed response from Ollama
	return strings.TrimSpace(ollamaResp.Response), nil
}

// Splits the input text into a specified number of chunks for processing
func splitTextIntoChunks(text string, numChunks int) []string {
	// Split the text into words and calculate the size of each chunk
	words := strings.Fields(text)
	totalWords := len(words)
	chunkSize := totalWords / numChunks

	var chunks []string
	// Create each chunk by slicing the words array and joining them back into a string
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

// Parses an SRT subtitle file and extracts the dialogue lines, ignoring timestamps and sequence numbers
func parseSRT(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	timestampRegex := regexp.MustCompile(`\d{2}:\d{2}:\d{2}`)
	var dialogueLines []string
	scanner := bufio.NewScanner(file)

	// Iterate through each line of the SRT file, filtering out empty lines, numeric sequence numbers, and timestamp lines
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || isNumeric(line) || timestampRegex.MatchString(line) {
			continue
		}
		dialogueLines = append(dialogueLines, line)
	}

	return strings.Join(dialogueLines, " "), scanner.Err()
}

// Checks if a string consists solely of numeric characters
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// Copies a file from the source path to the destination path
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
