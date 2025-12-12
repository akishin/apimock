package main

import (
	"encoding/json"
    "flag"
    "io"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "regexp"
    "strconv"
    "strings"
    "time"
)

var (
    mockDir     = flag.String("dir", "", "Mock directory (if empty, use config file or default)")
    port        = flag.String("port", "", "Port number (if empty, use config file or 8080)")
    showVersion = flag.Bool("version", false, "Show version information")
    _           = flag.Bool("v", false, "Show version information (short)")

    version = "v1.1.1"
    buildDate = "2025-12-12"

    configDir  string // Directory to use eventually
    configPort string // Port to use eventually
)

type Config struct {
    Dir  string      `json:"dir"`
    Port interface{} `json:"port"`
}

type MockResponse struct {
	Method  []string          `json:"method"`  // e.g. ["GET"], ["POST"], ["GET","POST"]
	Status  int               `json:"status"`  // Optional (default: 200)
	Delay   int               `json:"delay"`   // Milliseconds
	Headers map[string]string `json:"headers"` // Arbitrary custom headers
	Body    json.RawMessage   `json:"body"`    // Holds raw JSON
}

// Holds path parameters (corresponding to _ positions)
var currentPathParams []string

func main() {
	flag.Parse()

    if *showVersion || flag.Lookup("v").Value.(flag.Getter).Get().(bool) {
        println("apimock version " + version)
        println("Build date: " + buildDate)
        os.Exit(0)
    }

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
    if err := json.Unmarshal(data, &cfg); err != nil {
        log.Printf("[WARNING] Failed to parse config file '%s': %v", path, err)
        return
    }

    if cfg.Dir != "" {
        if strings.HasPrefix(cfg.Dir, "~/") {
            home, _ := os.UserHomeDir()
            configDir = filepath.Join(home, cfg.Dir[2:])
        } else {
            configDir = cfg.Dir
        }
    }
    if cfg.Port != nil {
        switch v := cfg.Port.(type) {
        case string:
            configPort = v
        case float64:
            configPort = strconv.Itoa(int(v))
        }
    }
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

	requestPath := strings.TrimPrefix(r.URL.Path, "/")

    filePath, pathParams := findBestMockFile(configDir, requestPath)
    currentPathParams = pathParams

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
		// Can expand {path.x} in headers as well
        v = replacePathParams(v)
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

	// Replace {path.x} with actual values
    replacedBody := replacePathParams(string(mock.Body))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(replacedBody))
}

// Replace path parameters
func replacePathParams(s string) string {
    re := regexp.MustCompile(`\{path\.(\d+)\}`)
    return re.ReplaceAllStringFunc(s, func(match string) string {
        if idxStr := re.FindStringSubmatch(match)[1]; idxStr != "" {
            if idx, err := strconv.Atoi(idxStr); err == nil && idx < len(currentPathParams) {
                return currentPathParams[idx]
            }
        }
        return match // Return as is if replacement fails
    })
}

// Find the best mock file (supports wildcards)
func findBestMockFile(baseDir, requestPath string) (string, []string) {
    requestParts := strings.Split(requestPath, "/")

    var bestMatch string
    var bestParams []string
    var bestScore int = -1 // The more _ there are, the lower the score (specific = fewer _ is prioritized)

    err := filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
        if err != nil || d.IsDir() {
            return err
        }
        if !strings.HasSuffix(path, ".json") {
            return nil
        }

        // Handle index.json
        rel := strings.TrimSuffix(path, ".json")
        if strings.HasSuffix(rel, "/index") {
            rel = strings.TrimSuffix(rel, "/index")
        }
        rel, _ = filepath.Rel(baseDir, rel)

        mockParts := strings.Split(rel, "/")

        if len(mockParts) != len(requestParts) {
            return nil
        }

        var params []string
        match := true
        underscoreCount := 0

        for i := range mockParts {
            if mockParts[i] == "_" {
                if requestParts[i] == "" {
                    match = false
                    break
                }
                params = append(params, requestParts[i])
                underscoreCount++
            } else if mockParts[i] != requestParts[i] {
                match = false
                break
            }
        }

        if match {
            score := len(requestParts) - underscoreCount // Fewer _ means higher score
            if score > bestScore {
                bestScore = score
                bestMatch = path
                bestParams = params
            }
        }

        return nil
    })

    if err != nil {
        log.Printf("Walk error: %v", err)
    }

    return bestMatch, bestParams
}

func respondJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
