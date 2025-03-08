package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// hasNoAttributes checks if the node has no specific attributes
func hasNoAttributes(n *html.Node, excludeAttrs []string) bool {
	for _, attr := range n.Attr {
		for _, excludeAttr := range excludeAttrs {
			if attr.Key == excludeAttr {
				return false
			}
		}
	}
	return true
}

// getTextContent gets all text content from a node and its children
func getTextContent(n *html.Node) string {
	var text string
	if n.Type == html.TextNode {
		text = n.Data
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text += getTextContent(c)
	}
	return strings.TrimSpace(text)
}

// isTargetElement checks if the node is one we want to keep
func isTargetElement(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}

	switch n.Data {
	case "ul":
		// Keep <ul> tags with no id or style
		return hasNoAttributes(n, []string{"id", "style"})
	case "p":
		// Keep <p> tags with no class and no "Copyright" in content
		if !hasNoAttributes(n, []string{"class", "style"}) {
			return false
		}
		content := getTextContent(n)
		return !strings.Contains(content, "Copyright")
	case "h3":
		// Keep <h3> tags with no class or id and no "Tokyo District Map" in content
		if !hasNoAttributes(n, []string{"class", "id"}) {
			return false
		}
		content := getTextContent(n)
		return !strings.Contains(content, "District Map")
	case "h1", "h2":
		// Keep heading tags with no class or id
		return hasNoAttributes(n, []string{"class", "id"})
	case "div":
		// Check for div with class="photogimg"
		for _, attr := range n.Attr {
			if attr.Key == "class" && attr.Val == "photogimg" {
				return true
			}
		}
	}
	return false
}

// renderNode converts a node back to HTML string
func renderNode(n *html.Node) string {
	var buf bytes.Buffer
	w := io.Writer(&buf)
	html.Render(w, n)
	return buf.String()
}

// extractContent processes the HTML and returns extracted content
func extractContent(doc *html.Node) []string {
	var extracted []string
	var f func(*html.Node)

	f = func(n *html.Node) {
		if isTargetElement(n) {
			extracted = append(extracted, renderNode(n))
		} else {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				f(c)
			}
		}
	}
	f(doc)
	return extracted
}

func formatDirName(name string) string {
	// Replace hyphens with spaces
	words := strings.Split(name, "-")
	// Capitalize first letter of each word
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

func checkAndCreateOutputDir(outputDir string) error {
	// Check if directory exists
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		// Create directory if it doesn't exist
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %v", err)
		}
		return nil
	}

	// Check if directory is empty
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("failed to read output directory: %v", err)
	}

	if len(entries) > 0 {
		fmt.Printf("Warning: Output directory %s is not empty\n", outputDir)
	}

	return nil
}

func processHTMLFile(inputFile string, outputDir string, category string, tag string) error {
	// Get the HTML file name without extension for the markdown file
	baseFileName := filepath.Base(inputFile)
	fileNameWithoutExt := strings.TrimSuffix(baseFileName, filepath.Ext(baseFileName))

	outputFile := strings.TrimSuffix(inputFile, ".html") + "_processed.html"
	mdOutputFile := filepath.Join(outputDir, fileNameWithoutExt+".md")

	// Read input file
	content, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("error reading file: %v", err)
	}

	// Parse HTML
	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return fmt.Errorf("error parsing HTML: %v", err)
	}

	// Extract content
	extractedContent := extractContent(doc)

	// Create new HTML document
	newHTML := `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
</head>
<body>
%s
</body>
</html>`

	// Join extracted content
	contentStr := strings.Join(extractedContent, "\n")

	// Create final HTML
	outputHTML := fmt.Sprintf(newHTML, contentStr)

	// Write to output file
	err = os.WriteFile(outputFile, []byte(outputHTML), 0644)
	if err != nil {
		return fmt.Errorf("error writing output file: %v", err)
	}

	fmt.Printf("Successfully extracted content to %s\n", outputFile)

	// Generate markdown using pandoc
	cmd := exec.Command("pandoc", "-f", "html", "-t", "markdown", outputFile, "-o", mdOutputFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error executing pandoc: %v", err)
	}

	fmt.Printf("Successfully converted to markdown: %s\n", mdOutputFile)

	// Read the markdown file
	mdContent, err := os.ReadFile(mdOutputFile)
	if err != nil {
		return fmt.Errorf("error reading markdown file: %v", err)
	}

	var result strings.Builder
	// Use the file name instead of parent directory name
	title := formatDirName(fileNameWithoutExt)

	// Build metadata
	result.WriteString("---\n")
	result.WriteString("title: \"" + title + "\"\n")
	result.WriteString("description: \"" + title + "\"\n")
	result.WriteString("meta_title: \"" + title + "\"\n")
	result.WriteString("author: " + "\"\"" + "\n")
	result.WriteString("date: " + time.Now().Format("2006-01-02") + "\n")
	result.WriteString("categories: [\"" + category + "\"]\n")
	result.WriteString("image: " + "\"\"" + "\n")
	result.WriteString("tags: [\"" + tag + "\"]\n")
	result.WriteString("draft: " + "false" + "\n")
	result.WriteString("---\n\n")

	mdText := string(mdContent)
	// First remove ::
	mdText = strings.ReplaceAll(mdText, "**::**", "")
	// Remove lines starting with :::
	lines := strings.Split(mdText, "\n")
	var filteredLines []string
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), ":::") {
			continue
		}

		// Replace link patterns
		line = strings.ReplaceAll(line, "(../", "(")
		line = strings.ReplaceAll(line, "/index.html)", ")")
		filteredLines = append(filteredLines, line)

	}
	mdText = strings.Join(filteredLines, "\n")
	// Process the entire text as one string
	for i := 0; i < len(string(mdText)); i++ {
		if string(mdText)[i] == '{' {
			// Find the closing brace
			j := i
			for j < len(string(mdText)) && string(mdText)[j] != '}' {
				j++
			}
			if j < len(string(mdText)) {
				// Skip past the closing brace
				i = j
				continue
			}
		}
		result.WriteByte(string(mdText)[i])
	}

	// Write the filtered content back to the file
	err = os.WriteFile(mdOutputFile, []byte(result.String()), 0644)
	if err != nil {
		return fmt.Errorf("error writing filtered markdown: %v", err)
	}

	fmt.Printf("Successfully processed: %s\n", inputFile)
	return nil
}

func main() {
	if len(os.Args) != 5 {
		fmt.Println("Usage: go run main.go <input_directory> <output_directory> <category> <tag>")
		os.Exit(1)
	}

	inputDir := os.Args[1]
	outputDir := os.Args[2]
	category := os.Args[3]
	tag := os.Args[4]

	// Check and create output directory
	if err := checkAndCreateOutputDir(outputDir); err != nil {
		fmt.Printf("Error with output directory: %v\n", err)
		os.Exit(1)
	}

	// Walk through directory
	err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Process only .html files
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".html") {
			if err := processHTMLFile(path, outputDir, category, tag); err != nil {
				fmt.Printf("Error processing %s: %v\n", path, err)
			}
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		os.Exit(1)
	}

	// clean processed files
	fmt.Println("Deleting processed files...")
	cmd := exec.Command("find", inputDir, "-type", "f", "-name", "*processed*", "-delete")
	if err := cmd.Run(); err != nil {
		fmt.Printf("error deleting processed files: %v\n", err)
		os.Exit(2)
	}
}
