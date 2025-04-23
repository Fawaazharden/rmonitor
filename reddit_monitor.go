package main

import (
	"context" // Needed for MongoDB operations
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp" // Added for sending email
	"os"       // Added for file operations and env vars
	"regexp"   // Added for regex matching
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref" // For pinging
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

var mongoURI = os.Getenv("MONGODB_URI") // MongoDB Connection String

// --- Internal Setup ---
var combinedSubreddits = strings.Join(subreddits, "+") // Keep this dynamic based on subreddits var
var postEndpoint = fmt.Sprintf("https://www.reddit.com/r/%s/new/.json?limit=100", combinedSubreddits)
var commentEndpoint = fmt.Sprintf("https://www.reddit.com/r/%s/comments/.json?limit=100", combinedSubreddits)

// Processed Item Tracking (MongoDB)
var mongoClient *mongo.Client
var processedItemsCollection *mongo.Collection

// Note: Persistence now handled by MongoDB

// HTTP Client with custom User-Agent
var httpClient = &http.Client{Timeout: 10 * time.Second} // Add a timeout
var userAgent = "GoRedditMonitor/1.0 (by /u/YourRedditUsername)" // CHANGE YourRedditUsername if possible

// --- MongoDB Setup ---

// setupMongoIndex ensures a unique index exists on the permalink field for efficient lookups.
// Run this in a goroutine from main to avoid blocking startup.
func setupMongoIndex() {
	if processedItemsCollection == nil {
		fmt.Println("WARN: Cannot setup index, MongoDB collection is nil.")
		return
	}
	// Create a unique index on the 'permalink' field
	indexModel := mongo.IndexModel{
		Keys:    map[string]interface{}{"permalink": 1}, // 1 for ascending
		Options: options.Index().SetUnique(true),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	indexName, err := processedItemsCollection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		// Log error, but maybe don't make it fatal? Index might exist or other issues.
		fmt.Printf("WARN: Could not create/verify MongoDB index (may already exist): %v\n", err)
	} else {
		fmt.Printf("MongoDB index '%s' on 'permalink' ensured.\n", indexName)
	}
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
	// newMatchesFound variable is less relevant now, DB handles state.
	for _, post := range posts {

		// --- Check if already processed (MongoDB Query) ---
		var result struct{} // We only care if a document is found, not its content
		ctxFind, cancelFind := context.WithTimeout(context.Background(), 5*time.Second)
		// FindOne returns ErrNoDocuments if not found
		err := processedItemsCollection.FindOne(ctxFind, map[string]interface{}{"permalink": post.Permalink}).Decode(&result)
		cancelFind() // Release context resources

		if err == nil {
			// Found the document, already processed
			continue
		} else if err != mongo.ErrNoDocuments {
			// An actual error occurred during the query
			fmt.Printf("Error checking MongoDB for post permalink %s: %v\n", post.Permalink, err)
			continue // Skip this post on DB error
		}
		// If err == mongo.ErrNoDocuments, it means it's NOT processed, so proceed.
		// --- End Check ---

		// Check for keywords (same as before)
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
				// --- Mark as processed (MongoDB Insert) ---
				ctxInsert, cancelInsert := context.WithTimeout(context.Background(), 5*time.Second)
				_, insertErr := processedItemsCollection.InsertOne(ctxInsert, map[string]interface{}{
					"permalink":    post.Permalink,
					"processed_at": time.Now(), // Store processing time
				})
				cancelInsert()

				if insertErr != nil {
					// Handle potential duplicate key error gracefully if index exists
					// but the check somehow missed it (less likely with FindOne)
					// Or other insertion errors
					// If it's a duplicate key error (code 11000), we can often ignore it.
					if mongo.IsDuplicateKeyError(insertErr) {
						fmt.Printf("Info: Attempted to insert duplicate permalink %s, already processed.\n", post.Permalink)
					} else {
						fmt.Printf("Error inserting processed post permalink %s into MongoDB: %v\n", post.Permalink, insertErr)
					}
				}
				// --- End Insert ---
			}
		}
		// No need to add to a map or save a file here
	}
	// No need for the final saveProcessedIDs call here
}

// processComments checks comments for keywords, sends email for new matches, and tracks processed IDs.
func processComments(comments []Comment) {
	// newMatchesFound variable is less relevant now, DB handles state.
	for _, comment := range comments {

		// --- Check if already processed (MongoDB Query) ---
		var result struct{}
		ctxFind, cancelFind := context.WithTimeout(context.Background(), 5*time.Second)
		err := processedItemsCollection.FindOne(ctxFind, map[string]interface{}{"permalink": comment.Permalink}).Decode(&result)
		cancelFind()

		if err == nil {
			continue // Already processed
		} else if err != mongo.ErrNoDocuments {
			fmt.Printf("Error checking MongoDB for comment permalink %s: %v\n", comment.Permalink, err)
			continue // Skip on DB error
		}
		// Not processed, continue
		// --- End Check ---

		// Check for keywords (same as before)
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
				// --- Mark as processed (MongoDB Insert) ---
				ctxInsert, cancelInsert := context.WithTimeout(context.Background(), 5*time.Second)
				_, insertErr := processedItemsCollection.InsertOne(ctxInsert, map[string]interface{}{
					"permalink":    comment.Permalink,
					"processed_at": time.Now(),
				})
				cancelInsert()

				if insertErr != nil {
					if mongo.IsDuplicateKeyError(insertErr) {
						fmt.Printf("Info: Attempted to insert duplicate permalink %s, already processed.\n", comment.Permalink)
					} else {
						fmt.Printf("Error inserting processed comment permalink %s into MongoDB: %v\n", comment.Permalink, insertErr)
					}
				}
				// --- End Insert ---
			}
		}
	}
	// No need for the final saveProcessedIDs call here
}

func main() {
	fmt.Println("Starting Reddit keyword monitor...")

	// --- Configuration Validation ---
	if gmailUser == "" || gmailAppPassword == "" || recipientEmail == "" {
		fmt.Println("FATAL: Email environment variables (GMAIL_USER, GMAIL_APP_PASSWORD, RECIPIENT_EMAIL) must be set.")
		os.Exit(1)
	}
	if mongoURI == "" {
		fmt.Println("FATAL: MONGODB_URI environment variable must be set.")
		os.Exit(1)
	}

	// --- Connect to MongoDB ---
	var err error
	clientOptions := options.Client().ApplyURI(mongoURI)
	ctxConnect, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelConnect()
	mongoClient, err = mongo.Connect(ctxConnect, clientOptions)
	if err != nil {
		fmt.Printf("FATAL: Unable to connect to MongoDB: %v\n", err)
		os.Exit(1)
	}

	// Ping the primary node to verify connection
	ctxPing, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPing()
	err = mongoClient.Ping(ctxPing, readpref.Primary())
	if err != nil {
		fmt.Printf("FATAL: MongoDB ping failed: %v\n", err)
		// Attempt to disconnect before exiting
		_ = mongoClient.Disconnect(context.Background())
		os.Exit(1)
	}
	fmt.Println("Successfully connected to MongoDB.")

	// Get collection handle
	// TODO: Consider making DB name and Collection name configurable via Env Vars too
	processedItemsCollection = mongoClient.Database("reddit_monitor").Collection("processed_items")

	// Ensure index exists (run in background)
	go setupMongoIndex()

	// Optional: Graceful shutdown handling
	// Setup signal catching for SIGINT and SIGTERM
	// sigs := make(chan os.Signal, 1)
	// signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	// go func() {
	// 	sig := <-sigs
	// 	fmt.Println()
	// 	fmt.Println("Received signal:", sig)
	// 	fmt.Println("Disconnecting from MongoDB...")
	// 	ctxDisconnect, cancelDisconnect := context.WithTimeout(context.Background(), 10*time.Second)
	// 	defer cancelDisconnect()
	// 	if err := mongoClient.Disconnect(ctxDisconnect); err != nil {
	// 		fmt.Printf("Error during MongoDB disconnect: %v\n", err)
	// 	}
	// 	fmt.Println("MongoDB disconnected. Exiting.")
	// 	os.Exit(0)
	// }()


	fmt.Println("--- Configuration ---")
	fmt.Println("Monitoring subreddits:", subreddits)
	fmt.Println("Looking for keywords:", keywords)
	fmt.Println("Sending notifications to:", recipientEmail)
	fmt.Println("Persistence: MongoDB")
	fmt.Println("---------------------")

	// Remove old file loading/saving logic

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