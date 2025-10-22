package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
)

// result holds the outcome of a single HTTP fetch.
type result struct {
	body string
	err  error
}

// fetchURL fetches the content of a given URL and sends the result to a channel.
func fetchURL(url string, ch chan<- result, wg *sync.WaitGroup) {
	defer wg.Done()

	resp, err := http.Get(url)
	if err != nil {
		ch <- result{err: fmt.Errorf("failed to get %s: %w", url, err)}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ch <- result{err: fmt.Errorf("bad status for %s: %s", url, resp.Status)}
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ch <- result{err: fmt.Errorf("failed to read body from %s: %w", url, err)}
		return
	}

	ch <- result{body: string(body)}
}

// urlList is a custom flag.Value type to allow multiple URL flags.
type urlList []string

// String is the method to format the value of the flag.
func (u *urlList) String() string {
	return fmt.Sprintf("%v", *u)
}

// Set is the method to set the value of the flag. It appends the value to the slice.
func (u *urlList) Set(value string) error {
	*u = append(*u, value)
	return nil
}

// aggregatorHandler fetches content from multiple URLs, concatenates their bodies, and writes the result back.
func aggregatorHandler(w http.ResponseWriter, r *http.Request, urls []string) {
	log.Printf("Received request for %s from %s, fetching from %v", r.URL.Path, r.RemoteAddr, urls)

	if len(urls) == 0 {
		http.Error(w, "No upstream URLs configured.", http.StatusInternalServerError)
		return
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(urls)) // Buffer size matches the number of URLs

	wg.Add(len(urls)) // Add count for each URL
	for _, u := range urls {
		go fetchURL(u, ch, &wg)
	}

	// Wait for both fetch operations to complete, then close the channel.
	go func() {
		wg.Wait()
		close(ch)
	}()

	var concatenatedBody string
	var errors []error

	// Read results from the channel.
	for res := range ch {
		if res.err != nil {
			log.Printf("Error fetching URL: %v", res.err)
			errors = append(errors, res.err)
			continue
		}
		concatenatedBody += res.body
	}

	// If there were any errors during fetching, return an internal server error.
	if len(errors) > 0 {
		http.Error(w, "Failed to fetch one or more upstream services.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, concatenatedBody)
}

func main() {
	// Define a command-line flag for the port.
	port := flag.Int("port", 8080, "Port for the HTTP server to listen on")

	// Define a custom flag to allow multiple URLs.
	var urls urlList
	flag.Var(&urls, "url", "URL to fetch from (can be specified multiple times)")

	flag.Parse()

	// If no URLs are provided via flags, use the original defaults.
	if len(urls) == 0 {
		urls = []string{"http://localhost:1234", "http://localhost:5678"}
		log.Printf("No URLs specified via -url flag, using defaults: %v", urls)
	} else {
		log.Printf("Configured to fetch from URLs: %v", urls)
	}

	// Register the handler function for the root path.
	// Use a closure to pass the configured URLs to the handler.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		aggregatorHandler(w, r, urls)
	})

	addr := fmt.Sprintf(":%d", *port) // Corrected: addr should be defined after port is parsed
	log.Printf("Starting server on %s", addr)

	// Start the HTTP server.
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
