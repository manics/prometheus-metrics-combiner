package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// TestAggregatorHandler tests the main aggregator handler logic.
func TestAggregatorHandler(t *testing.T) {
	// Mock upstream server 1
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "metric_a 1")
		fmt.Fprintln(w, "metric_b 2")
	}))
	defer server1.Close()

	// Mock upstream server 2
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "metric_c 3")
		fmt.Fprintln(w, "another_metric 4")
	}))
	defer server2.Close()

	// Mock a server that will fail
	server3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server3.Close()

	testCases := []struct {
		name           string
		urls           []string
		prefixes       []string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Two healthy upstreams, no filter",
			urls:           []string{server1.URL, server2.URL},
			prefixes:       []string{},
			expectedStatus: http.StatusOK,
			expectedBody:   "metric_a 1\nmetric_b 2\nmetric_c 3\nanother_metric 4\n",
		},
		{
			name:           "Two healthy upstreams, with prefix filter",
			urls:           []string{server1.URL, server2.URL},
			prefixes:       []string{"metric_"},
			expectedStatus: http.StatusOK,
			expectedBody:   "metric_a 1\nmetric_b 2\nmetric_c 3\n",
		},
		{
			name:           "Two healthy upstreams, with multiple prefix filters",
			urls:           []string{server1.URL, server2.URL},
			prefixes:       []string{"metric_a", "another_"},
			expectedStatus: http.StatusOK,
			expectedBody:   "metric_a 1\nanother_metric 4\n",
		},
		{
			name:           "One healthy, one failing upstream",
			urls:           []string{server1.URL, server3.URL},
			prefixes:       []string{},
			expectedStatus: http.StatusOK,
			expectedBody:   "metric_a 1\nmetric_b 2\n",
		},
		{
			name:           "All upstreams failing",
			urls:           []string{server3.URL, "http://localhost:12345"}, // one 500, one unreachable
			prefixes:       []string{},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Failed to fetch one or more upstream services.\n",
		},
		{
			name:           "No upstreams configured",
			urls:           []string{},
			prefixes:       []string{},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "No upstream URLs configured.\n",
		},
	}

	verbose := false
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/metrics", nil)
			rr := httptest.NewRecorder()

			aggregatorHandler(rr, req, tc.urls, tc.prefixes, &verbose)

			if status := rr.Code; status != tc.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tc.expectedStatus)
			}

			// For successful cases, we can't guarantee order, so we check for presence of lines.
			if tc.expectedStatus == http.StatusOK {
				body := rr.Body.String()
				expectedLines := strings.Split(strings.TrimSpace(tc.expectedBody), "\n")
				for _, line := range expectedLines {
					if !strings.Contains(body, line) {
						t.Errorf("handler response body does not contain expected line '%s'. Body:\n%s", line, body)
					}
				}
			} else {
				// For error cases, we can check the exact body.
				if body := rr.Body.String(); body != tc.expectedBody {
					t.Errorf("handler returned unexpected body: got '%v' want '%v'", body, tc.expectedBody)
				}
			}
		})
	}
}

// TestFetchURL tests the URL fetching logic in isolation.
func TestFetchURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/success" {
			fmt.Fprint(w, "ok")
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Run("Successful fetch", func(t *testing.T) {
		ch := make(chan result, 1)
		var wg sync.WaitGroup
		wg.Add(1)

		go fetchURL(server.URL+"/success", ch, &wg)
		wg.Wait()
		close(ch)

		res := <-ch
		if res.err != nil {
			t.Errorf("expected no error, but got: %v", res.err)
		}
		if res.body != "ok" {
			t.Errorf("expected body 'ok', but got: '%s'", res.body)
		}
	})

	t.Run("Failed fetch with bad status", func(t *testing.T) {
		ch := make(chan result, 1)
		var wg sync.WaitGroup
		wg.Add(1)

		go fetchURL(server.URL+"/fail", ch, &wg)
		wg.Wait()
		close(ch)

		res := <-ch
		if res.err == nil {
			t.Error("expected an error, but got none")
		}
		if !strings.Contains(res.err.Error(), "bad status") {
			t.Errorf("error message should contain 'bad status', but got: %v", res.err)
		}
	})
}

// TestStringListFlag tests the custom flag type for handling multiple string values.
func TestStringListFlag(t *testing.T) {
	var sl stringList
	if err := sl.Set("value1"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := sl.Set("value2"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	expected := stringList{"value1", "value2"}
	if !reflect.DeepEqual(sl, expected) {
		t.Errorf("stringList has wrong value: got %v, want %v", sl, expected)
	}

	expectedString := "[value1 value2]"
	if sl.String() != expectedString {
		t.Errorf("String() returned wrong value: got '%s', want '%s'", sl.String(), expectedString)
	}
}
