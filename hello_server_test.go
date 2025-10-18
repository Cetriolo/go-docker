package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	// Test case with name parameter
	reqWithParam, err := http.NewRequest("GET", "/?name=Test", nil)
	if err != nil {
		t.Fatal(err)
	}

	rrWithParam := httptest.NewRecorder()
	handler(rrWithParam, reqWithParam)

	if status := rrWithParam.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	expectedWithParam := "Hello, Test\n"
	if rrWithParam.Body.String() != expectedWithParam {
		t.Errorf("handler returned unexpected body: got %v want %v", rrWithParam.Body.String(), expectedWithParam)
	}

	// Test case without name parameter
	reqWithoutParam, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rrWithoutParam := httptest.NewRecorder()
	handler(rrWithoutParam, reqWithoutParam)

	if status := rrWithoutParam.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	expectedWithoutParam := "Hello, Guest\n"
	if rrWithoutParam.Body.String() != expectedWithoutParam {
		t.Errorf("handler returned unexpected body: got %v want %v", rrWithoutParam.Body.String(), expectedWithoutParam)
	}
}

func TestGetClientIP(t *testing.T) {
	// X-Forwarded-For should take precedence and first IP returned
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	req.RemoteAddr = "9.9.9.9:1234"
	if ip := getClientIP(req); ip != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %s", ip)
	}

	// X-Real-Ip used if X-Forwarded-For empty
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("X-Real-Ip", "2.2.2.2")
	req2.RemoteAddr = "9.9.9.9:1234"
	if ip := getClientIP(req2); ip != "2.2.2.2" {
		t.Errorf("expected 2.2.2.2, got %s", ip)
	}

	// fallback to RemoteAddr (without port)
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.RemoteAddr = "3.3.3.3:5678"
	if ip := getClientIP(req3); ip != "3.3.3.3" {
		t.Errorf("expected 3.3.3.3, got %s", ip)
	}

	// if RemoteAddr is not parseable, return as-is
	req4 := httptest.NewRequest("GET", "/", nil)
	req4.RemoteAddr = "bad-addr"
	if ip := getClientIP(req4); ip != "bad-addr" {
		t.Errorf("expected bad-addr, got %s", ip)
	}
}

func TestEchoHandler_Get(t *testing.T) {
	// GET with msg
	req := httptest.NewRequest("GET", "/echo?msg=hello", nil)
	rr := httptest.NewRecorder()
	echoHandler(rr, req)

	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("unexpected content type: %s", ct)
	}
	if rr.Body.String() != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", rr.Body.String())
	}

	// GET without msg
	req2 := httptest.NewRequest("GET", "/echo", nil)
	rr2 := httptest.NewRecorder()
	echoHandler(rr2, req2)
	if rr2.Body.String() != "no message\n" {
		t.Errorf("expected 'no message\\n', got %q", rr2.Body.String())
	}
}

func TestEchoHandler_Post(t *testing.T) {
	// POST with body
	body := []byte("posted-data")
	req := httptest.NewRequest("POST", "/echo", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	echoHandler(rr, req)

	if rr.Body.String() != string(body) {
		t.Errorf("expected %q, got %q", string(body), rr.Body.String())
	}

	// POST empty body
	req2 := httptest.NewRequest("POST", "/echo", bytes.NewReader([]byte{}))
	rr2 := httptest.NewRecorder()
	echoHandler(rr2, req2)
	if rr2.Body.String() != "empty body\n" {
		t.Errorf("expected 'empty body\\n', got %q", rr2.Body.String())
	}
}

func TestHeadersHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Test-Header", "val1")
	rr := httptest.NewRecorder()
	headersHandler(rr, req)

	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("unexpected content type: %s", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "X-Test-Header: val1") {
		t.Errorf("expected header line in response, got %q", body)
	}
}

func TestClientIPHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "4.4.4.4:9999"
	rr := httptest.NewRecorder()
	clientIPHandler(rr, req)

	if rr.Body.String() != "4.4.4.4\n" {
		t.Errorf("expected '4.4.4.4\\n', got %q", rr.Body.String())
	}
}

func TestInfoHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/info", nil)
	req.RemoteAddr = "5.5.5.5:1111"
	req.Header.Set("Accept-Language", "en-US")
	rr := httptest.NewRecorder()
	infoHandler(rr, req)

	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("unexpected content type: %s", ct)
	}

	var info map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&info); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if info["client_ip"] != "5.5.5.5" {
		t.Errorf("expected client_ip 5.5.5.5, got %s", info["client_ip"])
	}
	if info["accept_language"] != "en-US" {
		t.Errorf("expected accept_language en-US, got %s", info["accept_language"])
	}
	if info["method"] != "GET" {
		t.Errorf("expected method GET, got %s", info["method"])
	}
	if info["path"] != "/info" {
		t.Errorf("expected path /info, got %s", info["path"])
	}
}

func TestUserAgentHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/agent", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/90.0")
	req.RemoteAddr = "6.6.6.6:2222"
	rr := httptest.NewRecorder()
	userAgentHandler(rr, req)

	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("unexpected content type: %s", ct)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if resp["browser"] != "Chrome" {
		t.Errorf("expected browser Chrome, got %s", resp["browser"])
	}
	if resp["os"] != "Windows" {
		t.Errorf("expected os Windows, got %s", resp["os"])
	}
	if resp["client_ip"] != "6.6.6.6" {
		t.Errorf("expected client_ip 6.6.6.6, got %s", resp["client_ip"])
	}
	if resp["user_agent"] == "" {
		t.Errorf("expected non-empty user_agent")
	}
}
