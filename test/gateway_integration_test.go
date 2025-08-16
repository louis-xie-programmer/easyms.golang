package test

import (
	"net/http"
	"testing"
)

func TestGatewayToServer1(t *testing.T) {
	resp, err := http.Get("http://localhost:10001/api/server1/hello")
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestGatewayToServer2(t *testing.T) {
	resp, err := http.Get("http://localhost:10001/api/server2/hello")
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}
