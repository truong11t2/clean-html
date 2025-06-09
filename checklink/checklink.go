package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3"

// Global cache to store checked links and their statuses
var checkedLinksCache = make(map[string]string)

// writeResults writes the checking results to a file
func writeResults(url string, brokenLinks map[string]string, writer *bufio.Writer) error {
	_, err := writer.WriteString(fmt.Sprintf("\n=== Checking URL: %s ===\n", url))
	if err != nil {
		return err
	}

	if len(brokenLinks) > 0 {
		_, err = writer.WriteString("Broken links found:\n")
		if err != nil {
			return err
		}
		for link, status := range brokenLinks {
			_, err = writer.WriteString(fmt.Sprintf("• %s - %s\n", link, status))
			if err != nil {
				return err
			}
		}
	} else {
		_, err = writer.WriteString("All links are working!\n")
		if err != nil {
			return err
		}
	}
	return writer.Flush()
}

func main() {
	// Get the input file path
	var inputFile string
	fmt.Print("📄 Enter the path to the file containing URLs: ")
	fmt.Scanln(&inputFile)
	inputFile = strings.TrimSpace(inputFile)

	// Create output file with input filename appended
	inputBaseName := filepath.Base(inputFile)
	outputFile := fmt.Sprintf("%s_check_results.txt", inputBaseName)
	file, err := os.Create(outputFile)
	if err != nil {
		fmt.Printf("❌ Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	_, err = writer.WriteString("=== Link Check Results ===\n")
	if err != nil {
		fmt.Printf("❌ Error writing to output file: %v\n", err)
		os.Exit(1)
	}

	// Open and read the input file
	input, err := os.Open(inputFile)
	if err != nil {
		fmt.Printf("❌ Error opening input file: %v\n", err)
		os.Exit(1)
	}
	defer input.Close()

	scanner := bufio.NewScanner(input)
	var urls []string
	for scanner.Scan() {
		url := strings.TrimSpace(scanner.Text())
		if url != "" {
			urls = append(urls, url)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("❌ Error reading input file: %v\n", err)
		os.Exit(1)
	}

	if len(urls) == 0 {
		fmt.Println("⚠️ No URLs found in the file")
		return
	}

	fmt.Printf("📋 Found %d URLs to check\n", len(urls))
	fmt.Printf("📝 Results will be saved to: %s\n", outputFile)

	// Process each URL
	for i, url := range urls {
		fmt.Printf("\n🔍 Processing URL %d/%d: %s\n", i+1, len(urls), url)

		links, err := getLinks(url)
		if err != nil {
			fmt.Printf("❌ Error fetching links: %v\n", err)
			writer.WriteString(fmt.Sprintf("\n=== Checking URL: %s ===\nError: %v\n", url, err))
			writer.Flush()
			continue
		}

		if len(links) == 0 {
			fmt.Println("⚠️ No links found or couldn't access the page")
			writer.WriteString(fmt.Sprintf("\n=== Checking URL: %s ===\nNo links found or couldn't access the page\n", url))
			writer.Flush()
			continue
		}

		fmt.Printf("🔗 Found %d links to check\n", len(links))
		fmt.Println("\n⚡ Checking links...\n")

		brokenLinks := checkLinks(links)

		if len(brokenLinks) > 0 {
			fmt.Println("\n🔥 Broken links found:")
			for link, status := range brokenLinks {
				fmt.Printf("• %s - %s\n", link, status)
			}
		} else {
			fmt.Println("\n🎉 All links are working!")
		}

		// Write results to file
		if err := writeResults(url, brokenLinks, writer); err != nil {
			fmt.Printf("❌ Error writing results to file: %v\n", err)
		}
	}

	fmt.Printf("\n✅ Check complete! Results have been saved to: %s\n", outputFile)
}

func getLinks(urlStr string) ([]string, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	client := http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	baseURL := resp.Request.URL
	var links []string

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href := attr.Val

					// Handle "../" links by going back two levels
					if strings.HasPrefix(href, "../") {
						// Get the current path
						path := strings.TrimSuffix(baseURL.Path, "/")
						// Split path into components
						parts := strings.Split(path, "/")
						// Remove last two components (go back two levels)
						if len(parts) > 2 {
							parts = parts[:len(parts)-2]
							// Reconstruct the path
							newPath := strings.Join(parts, "/")
							if !strings.HasSuffix(newPath, "/") {
								newPath += "/"
							}
							// Remove "../" from the href and append to new path
							href = newPath + strings.TrimPrefix(href, "../")
						}
					} else if !strings.Contains(href, "/") {
						// Handle links without "/" by going back one level
						path := strings.TrimSuffix(baseURL.Path, "/")
						parts := strings.Split(path, "/")
						if len(parts) > 1 {
							parts = parts[:len(parts)-1]
							newPath := strings.Join(parts, "/")
							if !strings.HasSuffix(newPath, "/") {
								newPath += "/"
							}
							href = newPath + href
						}
					}

					absoluteURL, err := baseURL.Parse(href)
					if err != nil {
						continue
					}
					// Skip links containing "categories"
					if strings.Contains(absoluteURL.String(), "categories") {
						continue
					}
					if absoluteURL.Scheme == "http" || absoluteURL.Scheme == "https" {
						links = append(links, absoluteURL.String())
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}

	f(doc)
	return links, nil
}

func checkLinks(links []string) map[string]string {
	broken := make(map[string]string)
	client := http.Client{
		Timeout: 10 * time.Second,
	}

	for _, link := range links {
		// Check if link was already checked
		if status, exists := checkedLinksCache[link]; exists {
			fmt.Printf("⏭️  %s - Using cached result: %s\n", link, status)
			if status != "HTTP 200" {
				broken[link] = status
			}
			continue
		}

		statusCode, err := checkLink(client, link)
		if err != nil {
			status := err.Error()
			broken[link] = status
			checkedLinksCache[link] = status
			fmt.Printf("❌ %s - %s\n", link, status)
			continue
		}

		status := fmt.Sprintf("HTTP %d", statusCode)
		checkedLinksCache[link] = status

		if statusCode == http.StatusNotFound {
			broken[link] = status
			fmt.Printf("❌ %s - %s\n", link, status)
		} else {
			fmt.Printf("✅ %s - %s\n", link, status)
		}
	}

	return broken
}

func checkLink(client http.Client, link string) (int, error) {
	req, err := http.NewRequest("HEAD", link, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusMethodNotAllowed {
		req.Method = "GET"
		resp, err = client.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
	}

	return resp.StatusCode, nil
}
