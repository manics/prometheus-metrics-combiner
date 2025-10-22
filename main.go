package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
)

const (
	url1 = "http://localhost:1234"
	url2 = "http://localhost:5678"
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

// 聚合器处理程序获取两个URL，连接它们的主体，并将其写回。
func aggregatorHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request for %s from %s", r.URL.Path, r.RemoteAddr)

	var wg sync.WaitGroup
	// We create a buffered channel to prevent goroutine leaks if the receiver
	// stops listening before all sends are done (e.g., due to an early error).
	ch := make(chan result, 2)

	wg.Add(2)
	go fetchURL(url1, ch, &wg)
	go fetchURL(url2, ch, &wg)

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
	flag.Parse()

	// Register the handler function for the root path.
	http.HandleFunc("/", aggregatorHandler)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting server on %s", addr)

	// Start the HTTP server.
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
