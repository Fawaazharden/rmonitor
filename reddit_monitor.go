package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp" // Added for sending email
	"os"       // Added for file operations
	"regexp"   // Added for regex matching
	"strings"
	"time"
)

// Post represents a Reddit post's relevant fields
type Post struct {
	Title      string  `json:"title"`
	Selftext   string  `json:"selftext"`
	Permalink  string  `json:"permalink"`
	CreatedUtc float64 `json:"created_utc"`
	Subreddit  string  `json:"subreddit"`
}

// Comment represents a Reddit comment's relevant fields
type Comment struct {
	Body       string  `json:"body"`
	Permalink  string  `json:"permalink"`
	CreatedUtc float64 `json:"created_utc"`
	Subreddit  string  `json:"subreddit"`
}

// PostResponse matches the Reddit API's post listing structure
type PostResponse struct {
	Data struct {
		Children []struct {
			Data Post `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// CommentResponse matches the Reddit API's comment listing structure
type CommentResponse struct {
	Data struct {
		Children []struct {
			Data Comment `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// --- Configuration ---
var subreddits = []string{"WholesaleRealestate", "WholesalingHouses", "realestateinvesting", "RealEstateTechnology"}
var keywords = []string{"VA","leads"}

// Email Configuration (Read from Environment Variables)
var gmailUser = os.Getenv("GMAIL_USER")
var gmailAppPassword = os.Getenv("GMAIL_APP_PASSWORD") // Use an App Password for Gmail
var recipientEmail = os.Getenv("RECIPIENT_EMAIL")

// --- Internal Setup ---
var combinedSubreddits = strings.Join(subreddits, "+") // Keep this dynamic based on subreddits var
var postEndpoint = fmt.Sprintf("https://www.reddit.com/r/%s/new/.json?limit=100", combinedSubreddits)
var commentEndpoint = fmt.Sprintf("https://www.reddit.com/r/%s/comments/.json?limit=100", combinedSubreddits)

// Processed Item Tracking
var processedIDs = make(map[string]bool) // Use Permalink as the key
var processedIDsFile = os.Getenv("PROCESSED_IDS_PATH") // Read path from env var

// Note: Timestamp tracking is removed as ID tracking is more robust for preventing duplicates.

// HTTP Client with custom User-Agent
var httpClient = &http.Client{Timeout: 10 * time.Second} // Add a timeout
var userAgent = "GoRedditMonitor/1.0 (by /u/YourRedditUsername)" // CHANGE YourRedditUsername if possible

// --- Persistence Functions ---

// loadProcessedIDs loads the set of processed item permalinks from a JSON file.
func loadProcessedIDs(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, start with an empty set (first run)
			fmt.Println("Processed IDs file not found, starting fresh.")
			processedIDs = make(map[string]bool)
			return nil
		}
		// Other read error
		return fmt.Errorf("error reading processed IDs file: %w", err)
	}

	// File exists, unmarshal the JSON data
	err = json.Unmarshal(data, &processedIDs)
	if err != nil {
		return fmt.Errorf("error unmarshalling processed IDs: %w", err)
	}
	fmt.Printf("Loaded %d processed IDs.\n", len(processedIDs))
	return nil
}

// saveProcessedIDs saves the current set of processed item permalinks to a JSON file.
func saveProcessedIDs(filename string) error {
	data, err := json.MarshalIndent(processedIDs, "", "  ") // Pretty print JSON
	if err != nil {
		return fmt.Errorf("error marshalling processed IDs: %w", err)
	}

	err = os.WriteFile(filename, data, 0644) // Write with standard permissions
	if err != nil {
		return fmt.Errorf("error writing processed IDs file: %w", err)
	}
	// fmt.Printf("Saved %d processed IDs.\n", len(processedIDs)) // Optional: Log saving
	return nil
}


// --- Email Sending ---

// sendEmail sends an email notification using configured Gmail credentials.
func sendEmail(subject, body string) error {
	// Validation happens in main() now to check env vars at startup

	// Set up authentication information.
	auth := smtp.PlainAuth("", gmailUser, gmailAppPassword, "smtp.gmail.com")

	// SMTP server configuration.
	smtpHost := "smtp.gmail.com"
	smtpPort := "587" // Standard TLS port for Gmail SMTP

	// Message formatting (RFC 822 style).
	to := []string{recipientEmail}
	// Note: Ensure correct line endings (\r\n) for email headers/body separation.
	msg := []byte("To: " + recipientEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"\r\n" + // Empty line separates headers from body
		body + "\r\n")

	// Send the email.
	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, gmailUser, to, msg)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	fmt.Println("Email sent successfully to", recipientEmail)
	return nil
}


// --- Reddit API Fetching ---

// fetchPosts retrieves the latest posts from the Reddit API using a custom User-Agent
func fetchPosts(endpoint string) ([]Post, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d %s", resp.StatusCode, resp.Status)
	}

	var response PostResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		// Consider logging the raw body here for debugging if JSON parsing fails
		return nil, fmt.Errorf("error decoding JSON response: %w", err)
	}

	posts := make([]Post, 0, len(response.Data.Children))
	for _, child := range response.Data.Children {
		posts = append(posts, child.Data)
	}
	return posts, nil
}

// fetchComments retrieves the latest comments from the Reddit API using a custom User-Agent
func fetchComments(endpoint string) ([]Comment, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d %s", resp.StatusCode, resp.Status)
	}

	var response CommentResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("error decoding JSON response: %w", err)
	}

	comments := make([]Comment, 0, len(response.Data.Children))
	for _, child := range response.Data.Children {
		comments = append(comments, child.Data)
	}
	return comments, nil
}

// findKeywords checks for whole word keyword matches in text (case-insensitive)
func findKeywords(text string, keywords []string) []string {
	found := []string{}
	// Convert text to lower case once for case-insensitive matching
	textLower := strings.ToLower(text)

	for _, keyword := range keywords {
		// Escape regex special characters in the keyword
		escapedKeyword := regexp.QuoteMeta(keyword)
		// Create a case-insensitive regex pattern with word boundaries
		// (?i) makes it case-insensitive, \b ensures whole word matching
		pattern := fmt.Sprintf(`(?i)\b%s\b`, escapedKeyword)
		re, err := regexp.Compile(pattern)
		if err != nil {
			// Handle regex compilation error, e.g., log it
			fmt.Printf("Error compiling regex for keyword '%s': %v\n", keyword, err)
			continue // Skip this keyword if regex is invalid
		}

		// Check if the pattern matches the lowercased text
		if re.MatchString(textLower) {
			found = append(found, keyword) // Add the original keyword (not lowercased)
		}
	}
	return found
}

// processPosts checks posts for keywords, sends email for new matches, and tracks processed IDs.
func processPosts(posts []Post) {
	newMatchesFound := false
	for _, post := range posts {
		// Check if this post has already been processed
		if _, exists := processedIDs[post.Permalink]; exists {
			continue // Skip already processed post
		}

		// Check for keywords
		text := post.Title + " " + post.Selftext
		found := findKeywords(text, keywords)

		if len(found) > 0 {
			// New match found!
			fmt.Printf("Found keywords %v in NEW post from r/%s: https://www.reddit.com%s\n",
				found, post.Subreddit, post.Permalink)

			// Format email content (link only)
			subject := fmt.Sprintf("Reddit Keyword Alert: Post in r/%s", post.Subreddit)
			body := fmt.Sprintf("Keywords %v found in post:\nhttps://www.reddit.com%s", found, post.Permalink)

			// Send email
			err := sendEmail(subject, body)
			if err != nil {
				fmt.Println("Error sending post notification email:", err)
				// Decide if you want to stop processing or just log the error
				// continue // Optional: Continue processing other posts even if email fails
			} else {
				// Mark as processed ONLY if email was sent successfully (or if you choose to mark anyway)
				processedIDs[post.Permalink] = true
				newMatchesFound = true
			}
		}
		// Note: We no longer track the max timestamp. We only care if the ID is new.
		// We also add the ID even if no keywords are found IF we want to avoid re-checking
		// non-matching posts in the future. For now, only adding matching ones.
		// If you want to avoid re-checking *all* posts fetched, uncomment the next line:
		// processedIDs[post.Permalink] = true
	}
	// Save IDs immediately after processing posts if new matches were added
	if newMatchesFound {
		err := saveProcessedIDs(processedIDsFile)
		if err != nil {
			fmt.Println("Error saving processed IDs after post processing:", err)
		}
	}
}

// processComments checks comments for keywords, sends email for new matches, and tracks processed IDs.
func processComments(comments []Comment) {
	newMatchesFound := false
	for _, comment := range comments {
		// Check if this comment has already been processed
		if _, exists := processedIDs[comment.Permalink]; exists {
			continue // Skip already processed comment
		}

		// Check for keywords
		found := findKeywords(comment.Body, keywords)

		if len(found) > 0 {
			// New match found!
			fmt.Printf("Found keywords %v in NEW comment from r/%s: https://www.reddit.com%s\n",
				found, comment.Subreddit, comment.Permalink)

			// Format email content (link only)
			subject := fmt.Sprintf("Reddit Keyword Alert: Comment in r/%s", comment.Subreddit)
			body := fmt.Sprintf("Keywords %v found in comment:\nhttps://www.reddit.com%s", found, comment.Permalink)

			// Send email
			err := sendEmail(subject, body)
			if err != nil {
				fmt.Println("Error sending comment notification email:", err)
				// continue // Optional: Continue processing other comments even if email fails
			} else {
				// Mark as processed ONLY if email was sent successfully
				processedIDs[comment.Permalink] = true
				newMatchesFound = true
			}
		}
		// Mark all checked comments as processed? Uncomment below if desired.
		// processedIDs[comment.Permalink] = true
	}
	// Save IDs immediately after processing comments if new matches were added
	if newMatchesFound {
		err := saveProcessedIDs(processedIDsFile)
		if err != nil {
			fmt.Println("Error saving processed IDs after comment processing:", err)
		}
	}
}

func main() {
	fmt.Println("Starting Reddit keyword monitor...")

	// --- Configuration Validation ---
	if gmailUser == "" || gmailAppPassword == "" || recipientEmail == "" {
		fmt.Println("FATAL: Email environment variables (GMAIL_USER, GMAIL_APP_PASSWORD, RECIPIENT_EMAIL) must be set.")
		os.Exit(1)
	}
	if processedIDsFile == "" {
		fmt.Println("WARN: PROCESSED_IDS_PATH environment variable not set, defaulting to 'processed_ids.json'")
		processedIDsFile = "processed_ids.json"
		// If you require the path to be set, uncomment the lines below:
		// fmt.Println("FATAL: PROCESSED_IDS_PATH environment variable must be set.")
		// os.Exit(1)
	}
	fmt.Println("--- Configuration ---")
	fmt.Println("Monitoring subreddits:", subreddits)
	fmt.Println("Looking for keywords:", keywords)
	fmt.Println("Sending notifications to:", recipientEmail)
	fmt.Println("Processed IDs file path:", processedIDsFile)
	fmt.Println("---------------------")

	// Load previously processed IDs
	err := loadProcessedIDs(processedIDsFile)
	if err != nil {
		fmt.Println("Error loading processed IDs, starting with empty set:", err)
		// Continue even if loading fails, will just start fresh
		processedIDs = make(map[string]bool)
	}

	// Initial save in case the file didn't exist and needs creation
	// This ensures the file exists for subsequent saves within the loop.
	// Only attempt save if path is valid or defaulted.
	err = saveProcessedIDs(processedIDsFile)
	if err != nil {
		fmt.Println("Error performing initial save of processed IDs file:", err)
		// Decide if this is critical enough to stop. For now, we continue.
	}


	for {
		fmt.Println("\nFetching new data at", time.Now().Format(time.RFC1123))
		// Fetch and process posts
		posts, err := fetchPosts(postEndpoint)
		if err != nil {
			fmt.Println("Error fetching posts:", err)
		} else {
			processPosts(posts)
		}

		// Fetch and process comments
		comments, err := fetchComments(commentEndpoint)
		if err != nil {
			fmt.Println("Error fetching comments:", err)
		} else {
			processComments(comments)
		}

		// Wait before the next iteration
		time.Sleep(5 * time.Minute)
	}
}