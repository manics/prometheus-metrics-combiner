package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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

// stringList is a custom flag.Value type to allow multiple string flags
type stringList []string

// String is the method to format the value of the flag.
func (s *stringList) String() string {
	return fmt.Sprintf("%v", *s)
}

// Set is the method to set the value of the flag. It appends the value to the slice.
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// aggregatorHandler fetches content from multiple URLs, concatenates their bodies, and writes the result back.
func aggregatorHandler(w http.ResponseWriter, r *http.Request, urls []string, prefixes []string, verbose *bool) {
	if *verbose {
		log.Printf("Received request for %s from %s, fetching from %v", r.URL.Path, r.RemoteAddr, urls)
	}

	if len(urls) == 0 {
		http.Error(w, "No upstream URLs configured.", http.StatusInternalServerError)
		return
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(urls))

	wg.Add(len(urls))
	for _, u := range urls {
		go fetchURL(u, ch, &wg)
	}

	// Wait for both fetch operations to complete, then close the channel.
	go func() {
		wg.Wait()
		close(ch)
	}()

	var concatenatedBody strings.Builder
	var errors []error

	// Read results from the channel.
	for res := range ch {
		if res.err != nil {
			log.Printf("Error fetching URL: %v", res.err)
			errors = append(errors, res.err)
			continue
		}

		if len(prefixes) == 0 {
			// If no prefixes are specified, concatenate the entire body
			concatenatedBody.WriteString(res.body)
		} else {
			// Otherwise, filter lines by prefix
			scanner := bufio.NewScanner(strings.NewReader(res.body))
			for scanner.Scan() {
				line := scanner.Text()
				for _, p := range prefixes {
					if strings.HasPrefix(line, p) {
						concatenatedBody.WriteString(line)
						concatenatedBody.WriteString("\n")
						break
					}
				}
			}
		}
	}

	// Return an error if all fetches failed, otherwise return partial results
	if len(errors) == len(urls) {
		http.Error(w, "Failed to fetch one or more upstream services.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, concatenatedBody.String())
}

func main() {
	port := flag.Int("port", 8080, "Port for the HTTP server to listen on")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")

	// Custom flags to allow multiple URLs and prefixes

	var urls stringList
	flag.Var(&urls, "url", "URL to fetch from (can be specified multiple times)")

	var prefixes stringList
	flag.Var(&prefixes, "prefix", "Prefix for lines to include in the output (can be specified multiple times). If no prefixes are given, all lines are included.")

	flag.Parse()

	if len(urls) == 0 {
		log.Fatal("Error: At least one upstream URL must be specified with the -url flag.")
	}

	log.Printf("Configured to fetch from URLs: %v", urls)
	if len(prefixes) > 0 {
		log.Printf("Configured to filter metrics by prefixes: %v", prefixes)
	} else {
		log.Println("No prefixes specified, all metrics will be included.")
	}

	// Register the handler function for root path
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		aggregatorHandler(w, r, urls, prefixes, verbose)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting server on %s", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
