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
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"gopkg.in/natefinch/lumberjack.v2"
)

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
}

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
