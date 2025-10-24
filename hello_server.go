package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Redis client
var rdb *redis.Client

func handler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	name := query.Get("name")
	if name == "" {
		name = "Guest"
	}
	log.Printf("Received request for %s\n", name)
	log.Printf(" %v\n", r)
	w.Write([]byte(fmt.Sprintf("Hello, %s\n", name)))
}

func main() {
	// Configure Logging
	LOG_FILE_LOCATION := os.Getenv("LOG_FILE_LOCATION")
	if LOG_FILE_LOCATION != "" {
		log.SetOutput(&lumberjack.Logger{
			Filename:   LOG_FILE_LOCATION,
			MaxSize:    500, // megabytes
			MaxBackups: 3,
			MaxAge:     28,   //days
			Compress:   true, // disabled by default
		})
	}

	// Initialize Redis
	initRedis()
	seedRedisData(context.Background())

	// Create Server and Route Handlers
	r := mux.NewRouter()

	r.HandleFunc("/", handler)
	registerExtraRoutes(r)
	srv := &http.Server{
		Handler:      r,
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start Server
	go func() {
		log.Println("Starting Server")
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	// Graceful Shutdown
	waitForShutdown(srv)
}

func waitForShutdown(srv *http.Server) {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	<-interruptChan

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	srv.Shutdown(ctx)

	log.Println("Shutting down")
	os.Exit(0)
}

func registerExtraRoutes(r *mux.Router) {
	r.HandleFunc("/info", infoHandler)
	r.HandleFunc("/agent", userAgentHandler)
	r.HandleFunc("/headers", headersHandler)
	r.HandleFunc("/ip", clientIPHandler)
	r.HandleFunc("/echo", echoHandler).Methods("GET", "POST")
	r.HandleFunc("/redis", redisHandler).Methods("GET") // New Redis route
}

// --- New Redis Functions ---

func initRedis() {
	// Configuration from environment variables
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "test-db-e2kuf-redis-master.test-db-e2kuf.svc.cluster.local:6379" // default
	}
	//password := os.Getenv("REDIS_PASSWORD") // no password by default
	dbStr := os.Getenv("REDIS_DB")
	if dbStr == "" {
		dbStr = "0" // default db
	}
	db, err := strconv.Atoi(dbStr)
	if err != nil {
		log.Fatalf("Invalid Redis DB number: %v", err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     "test-g6r4t-redis-master.test-g6r4t.svc.cluster.local:6379",
		Password: "cE0+mF2_sV3_cQ3+vT0-",
		DB:       db,
	})

	// Ping to check connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	log.Println("Connected to Redis successfully.")
}

// seedRedisData adds some predetermined data to Redis.
func seedRedisData(ctx context.Context) {
	log.Println("Seeding Redis with initial data...")
	err := rdb.Set(ctx, "app:name", "go-hello-server", 0).Err()
	if err != nil {
		log.Printf("Failed to seed data 'app:name': %v", err)
	}
	err = rdb.Set(ctx, "user:1:name", "Cetriolo", 0).Err()
	if err != nil {
		log.Printf("Failed to seed data 'user:1:name': %v", err)
	}
}

// redisHandler retrieves a value from Redis by key.
// Example: /redis?key=app:name
func redisHandler(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "Query parameter 'key' is required", http.StatusBadRequest)
		return
	}

	val, err := rdb.Get(r.Context(), key).Result()
	if err == redis.Nil {
		http.Error(w, fmt.Sprintf("Key '%s' not found", key), http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Failed to retrieve data from Redis", http.StatusInternalServerError)
		log.Printf("Redis GET error for key '%s': %v", key, err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, val)
}

// --- Existing Handlers ---

func getClientIP(r *http.Request) string {
	// Check common proxy headers first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple addresses, take the first
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xr := r.Header.Get("X-Real-Ip"); xr != "" {
		return xr
	}
	// Fallback to RemoteAddr (strip port)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	info := map[string]string{
		"client_ip":       getClientIP(r),
		"user_agent":      r.UserAgent(),
		"accept_language": r.Header.Get("Accept-Language"),
		"method":          r.Method,
		"path":            r.URL.Path,
		"protocol":        r.Proto,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(info)
}

func userAgentHandler(w http.ResponseWriter, r *http.Request) {
	ua := r.UserAgent()
	browser := "Unknown"
	switch {
	case strings.Contains(ua, "OPR") || strings.Contains(ua, "Opera"):
		browser = "Opera"
	case strings.Contains(ua, "Edg") || strings.Contains(ua, "Edge"):
		browser = "Edge"
	case strings.Contains(ua, "Chrome") && !strings.Contains(ua, "Chromium"):
		browser = "Chrome"
	case strings.Contains(ua, "Chromium"):
		browser = "Chromium"
	case strings.Contains(ua, "Firefox"):
		browser = "Firefox"
	case strings.Contains(ua, "Safari") && !strings.Contains(ua, "Chrome"):
		browser = "Safari"
	case strings.Contains(ua, "MSIE") || strings.Contains(ua, "Trident"):
		browser = "Internet Explorer"
	}

	osname := "Unknown"
	switch {
	case strings.Contains(ua, "Windows"):
		osname = "Windows"
	case strings.Contains(ua, "Macintosh"):
		osname = "macOS"
	case strings.Contains(ua, "Android"):
		osname = "Android"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		osname = "iOS"
	case strings.Contains(ua, "Linux"):
		osname = "Linux"
	}

	resp := map[string]string{
		"browser":    browser,
		"os":         osname,
		"user_agent": ua,
		"client_ip":  getClientIP(r),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

func headersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for k, v := range r.Header {
		fmt.Fprintf(w, "%s: %s\n", k, strings.Join(v, ", "))
	}
}

func clientIPHandler(w http.ResponseWriter, r *http.Request) {
	ip := getClientIP(r)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, ip)
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Method == http.MethodGet {
		msg := r.URL.Query().Get("msg")
		if msg == "" {
			fmt.Fprintln(w, "no message")
			return
		}
		fmt.Fprintln(w, msg)
		return
	}
	// POST: echo body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	if len(body) == 0 {
		fmt.Fprintln(w, "empty body")
		return
	}
	w.Write(body)
}
