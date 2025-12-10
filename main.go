package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type MockResponse struct {
	Method  []string          `json:"method"`  // e.g. ["GET"], ["POST"], ["GET","POST"]
	Status  int               `json:"status"`  // Optional (default: 200)
	Delay   int               `json:"delay"`   // Milliseconds
	Headers map[string]string `json:"headers"` // Arbitrary custom headers
	Body    json.RawMessage   `json:"body"`    // Holds raw JSON
}

var (
    mockDir = flag.String("dir", "", "Mock directory (if empty, use config file or default)")
    port    = flag.String("port", "", "Port number (if empty, use config file or 8080)")

    configDir  string // Directory to use eventually
    configPort string // Port to use eventually
)

type Config struct {
    Dir  string `json:"dir"`
    Port string `json:"port"`
}

func initConfig() {
    // Default values
    configDir = "mock"
    configPort = "8080"

    // 1. Load config from home directory
    loadConfigFromPath(os.ExpandEnv("$HOME/.apimockrc"))
    // 2. Load config from current directory (override)
    loadConfigFromPath(".apimockrc")

    // Override if command line arguments are specified
    if *mockDir != "" {
        configDir = *mockDir
    }
    if *port != "" {
        configPort = *port
    }

    // Final check
    if info, err := os.Stat(configDir); err != nil || !info.IsDir() {
        log.Fatalf("Mock directory '%s' not found. Please specify with --dir or write correct path in .apimockrc.", configDir)
    }
}

func loadConfigFromPath(path string) {
    data, err := os.ReadFile(path)
    if err != nil {
        return // Ignore if file does not exist
    }

    var cfg Config
    if json.Unmarshal(data, &cfg) != nil {
        return // Ignore parse failure
    }

    if cfg.Dir != "" {
        configDir = cfg.Dir
    }
    if cfg.Port != "" {
        configPort = cfg.Port
    }
}

func main() {
	flag.Parse()
	initConfig()

	log.Printf("[apimock] Starting -> http://localhost:%s", configPort)
    log.Printf("Mock directory: %s", configDir)
	
	entries, _ := os.ReadDir(configDir)
	for _, e := range entries {
		if e.IsDir() {
			log.Printf("  └─ 📁 %s/", e.Name())
		} else if strings.HasSuffix(e.Name(), ".json") {
			log.Printf("  ├─ 📄 %s", e.Name())
		}
	}
    log.Println("Press Ctrl+C to stop")

    http.HandleFunc("/", mockHandler)
	log.Fatal(http.ListenAndServe(":"+configPort, nil))
}

func mockHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("apimock server is running!"))
		return
	}

	// Allow all CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
    candidates := []string{
        filepath.Join(configDir, path+".json"),        // /users -> mock/users.json
        filepath.Join(configDir, path, "index.json"),  // /users -> mock/users/index.json
    }

    var filePath string
    for _, p := range candidates {
        if _, err := os.Stat(p); err == nil {
            filePath = p
            break
        }
    }

	// 404 if file not found
	if filePath == "" {
		respondJSON(w, 404, map[string]string{"error": "Not Found"})
		return
	}

	// Parse JSON file
	file, err := os.Open(filePath)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Server Error"})
		return
	}
	defer file.Close()

	data, _ := io.ReadAll(file)
	var mock MockResponse
	if err := json.Unmarshal(data, &mock); err != nil {
		// Parse failed -> return as raw JSON with 200 (compatibility with old method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(data)
		return
	}

	// Check method
	if len(mock.Method) > 0 {
		allowed := false
		for _, m := range mock.Method {
			if r.Method == m {
				allowed = true
				break
			}
		}
		if !allowed {
			respondJSON(w, 405, map[string]string{
				"error": "Method Not Allowed",
				"allow": strings.Join(mock.Method, ", "),
			})
			return
		}
	}

	// Handle delay
	if mock.Delay > 0 {
		time.Sleep(time.Duration(mock.Delay) * time.Millisecond)
	}

	// Set headers
	for k, v := range mock.Headers {
		w.Header().Set(k, v)
	}

	// status (default 200)
	status := mock.Status
	if status == 0 {
		status = 200
	}

	// If body is empty -> 204 or empty JSON
	if len(mock.Body) == 0 || string(mock.Body) == "null" {
		if status == 200 {
			status = 204
		}
		w.WriteHeader(status)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(mock.Body)
}

func respondJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
