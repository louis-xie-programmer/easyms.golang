package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"net/http"
)

const (
	// MockHeader is the header key to trigger a mock response.
	MockHeader = "X-Mock-Response"
)

// MockPlugin provides mock responses for testing and development.
type MockPlugin struct{}

// NewMockPlugin creates a new mock plugin.
func NewMockPlugin() *MockPlugin {
	return &MockPlugin{}
}

func (p *MockPlugin) Name() string {
	return "mock"
}

func (p *MockPlugin) Order() int {
	// Should be one of the first plugins to run.
	return 5
}

func (p *MockPlugin) Execute(ctx *plugin.Context) {
	mockHeaderValue := ctx.Request.Header.Get(MockHeader)

	if mockHeaderValue != "true" {
		// If the header is not present or not "true", continue to the next plugin.
		ctx.Next()
		return
	}

	// Header is present, so we provide a mock response and stop the chain.
	ctx.ResponseWriter.Header().Set("Content-Type", "application/json")
	ctx.ResponseWriter.WriteHeader(http.StatusOK)
	ctx.ResponseWriter.Write([]byte(`{"message": "This is a mock response from the gateway."}`))

	// Do NOT call ctx.Next(), effectively short-circuiting the request chain.
}
