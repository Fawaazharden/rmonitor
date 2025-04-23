

### Step 1: Set Up Your Go Environment
- Ensure Go is installed on your system. Download it from [https://golang.org/dl/](https://golang.org/dl/) if needed.
- Verify installation by running `go version` in your terminal.

### Step 2: Create a New Go File
- Create a file named `reddit_monitor.go` in a directory of your choice.
- Open it in a text editor or IDE (e.g., VS Code, GoLand).

### Step 3: Import Required Packages
- Add the necessary Go packages to handle HTTP requests, JSON parsing, and timing.

### Step 4: Define Data Structures
- Define structs to match the Reddit API's JSON response for posts and comments, including fields like title, text, timestamp, and subreddit.

### Step 5: Specify Subreddits and Keywords
- Define lists of subreddits and keywords you want to monitor.

### Step 6: Construct API Endpoints
- Combine subreddits into a single string and create Reddit API endpoints for fetching posts and comments.

### Step 7: Initialize Timestamp Tracking
- Use variables to track the last processed post and comment timestamps to avoid duplicates.

### Step 8: Implement Fetch Functions
- Write functions to retrieve posts and comments from the Reddit API and parse the JSON responses.

### Step 9: Create a Keyword Matching Function
- Develop a function to check if any keywords appear in text, making the search case-insensitive.

### Step 10: Process Posts and Comments
- Write functions to process fetched data, checking for keywords and logging matches.

### Step 11: Set Up the Monitoring Loop
- Create a main loop that periodically fetches and processes data from Reddit.

### Step 12: Run the Program
- Compile and run the program to start monitoring.

---

Here’s the complete implementation:

```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
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

// Subreddits and keywords to monitor
var subreddits = []string{"golang", "programming", "technology"}
var keywords = []string{"golang", "reddit", "monitoring"}

// Combine subreddits for API calls
var combinedSubreddits = strings.Join(subreddits, "+")

// Define API endpoints
var postEndpoint = fmt.Sprintf("https://www.reddit.com/r/%s/new/.json?limit=100", combinedSubreddits)
var commentEndpoint = fmt.Sprintf("https://www.reddit.com/r/%s/comments/.json?limit=100", combinedSubreddits)

// Track last processed timestamps
var lastPostTimestamp float64
var lastCommentTimestamp float64

// fetchPosts retrieves the latest posts from the Reddit API
func fetchPosts(endpoint string) ([]Post, error) {
    resp, err := http.Get(endpoint)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    var response PostResponse
    err = json.NewDecoder(resp.Body).Decode(&response)
    if err != nil {
        return nil, err
    }
    posts := make([]Post, 0, len(response.Data.Children))
    for _, child := range response.Data.Children {
        posts = append(posts, child.Data)
    }
    return posts, nil
}

// fetchComments retrieves the latest comments from the Reddit API
func fetchComments(endpoint string) ([]Comment, error) {
    resp, err := http.Get(endpoint)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    var response CommentResponse
    err = json.NewDecoder(resp.Body).Decode(&response)
    if err != nil {
        return nil, err
    }
    comments := make([]Comment, 0, len(response.Data.Children))
    for _, child := range response.Data.Children {
        comments = append(comments, child.Data)
    }
    return comments, nil
}

// findKeywords checks for keyword matches in text (case-insensitive)
func findKeywords(text string, keywords []string) []string {
    textLower := strings.ToLower(text)
    found := []string{}
    for _, keyword := range keywords {
        if strings.Contains(textLower, strings.ToLower(keyword)) {
            found = append(found, keyword)
        }
    }
    return found
}

// processPosts checks posts for keywords and updates the timestamp
func processPosts(posts []Post) {
    maxTimestamp := lastPostTimestamp
    for _, post := range posts {
        if post.CreatedUtc > lastPostTimestamp {
            text := post.Title + " " + post.Selftext
            found := findKeywords(text, keywords)
            if len(found) > 0 {
                fmt.Printf("Found keywords %v in post from r/%s at %s: https://www.reddit.com%s\n",
                    found, post.Subreddit, time.Unix(int64(post.CreatedUtc), 0), post.Permalink)
            }
            if post.CreatedUtc > maxTimestamp {
                maxTimestamp = post.CreatedUtc
            }
        }
    }
    lastPostTimestamp = maxTimestamp
}

// processComments checks comments for keywords and updates the timestamp
func processComments(comments []Comment) {
    maxTimestamp := lastCommentTimestamp
    for _, comment := range comments {
        if comment.CreatedUtc > lastCommentTimestamp {
            found := findKeywords(comment.Body, keywords)
            if len(found) > 0 {
                fmt.Printf("Found keywords %v in comment from r/%s at %s: https://www.reddit.com%s\n",
                    found, comment.Subreddit, time.Unix(int64(comment.CreatedUtc), 0), comment.Permalink)
            }
            if comment.CreatedUtc > maxTimestamp {
                maxTimestamp = comment.CreatedUtc
            }
        }
    }
    lastCommentTimestamp = maxTimestamp
}

func main() {
    fmt.Println("Starting Reddit keyword monitor...")
    for {
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
```

---

### Running the Program
- Open a terminal in the directory containing `reddit_monitor.go`.
- Run the program with:
  ```bash
  go run reddit_monitor.go
  ```
- The program will start monitoring and log any keyword matches to the console.

### Customization and Notes
- **Subreddits and Keywords**: Modify the `subreddits` and `keywords` slices to monitor different subreddits or terms.
- **Interval**: The program checks every 5 minutes (`time.Sleep(5 * time.Minute)`). Adjust this value as needed, but respect Reddit’s API rate limit of 60 requests per minute. With two requests per iteration (posts and comments), 5 minutes ensures compliance.
- **Limitations**: The program fetches up to 100 items per request. If more than 100 new posts or comments appear between checks, some may be missed. For a more robust solution, implement pagination with the `after` parameter or refer to advanced tools like KWatch.io.
- **Error Handling**: Basic error logging is included. Enhance this for production use (e.g., retries, backoff).
