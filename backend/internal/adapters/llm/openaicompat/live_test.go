package openaicompat

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// TestLiveOpenRouter is the one test that spends money. It is skipped unless
// PROTEAN_TEST_OPENROUTER=1 and a key is present, so `go test ./...` never
// reaches the network.
//
//	PROTEAN_TEST_OPENROUTER=1 OPENROUTER_API_KEY=... go test ./internal/adapters/llm/openaicompat -run Live -v
func TestLiveOpenRouter(t *testing.T) {
	if os.Getenv("PROTEAN_TEST_OPENROUTER") != "1" {
		t.Skip("set PROTEAN_TEST_OPENROUTER=1 to run the live provider smoke test")
	}
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY is not set")
	}

	model := os.Getenv("PROTEAN_TEST_OPENROUTER_MODEL")
	if model == "" {
		model = "xiaomi/mimo-v2.5-pro"
	}

	client, err := New(Options{
		BaseURL:  "https://openrouter.ai/api/v1",
		APIKey:   apiKey,
		Model:    model,
		SiteURL:  os.Getenv("OPENROUTER_SITE_URL"),
		SiteName: os.Getenv("OPENROUTER_SITE_NAME"),
		Logger:   discardLogger(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	stream, err := client.StreamChat(ctx, ports.ChatRequest{
		Messages: []ports.ChatMessage{
			{Role: ports.RoleUser, Content: `Reply with exactly one word: "pong".`},
		},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer stream.Close()

	var (
		text  strings.Builder
		usage ports.Usage
		done  bool
	)
	for {
		ev, ok := stream.Next()
		if !ok {
			break
		}
		switch ev.Kind {
		case ports.StreamText:
			text.WriteString(ev.Text)
		case ports.StreamDone:
			usage, done = ev.Usage, true
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}

	if !done {
		t.Fatal("stream ended without a done event")
	}
	if strings.TrimSpace(text.String()) == "" {
		t.Error("no text came back")
	}
	if usage.InputTokens <= 0 || usage.OutputTokens <= 0 {
		t.Errorf("usage = %+v, want both counts above zero", usage)
	}
	t.Logf("model=%s text=%q usage=%+v", model, strings.TrimSpace(text.String()), usage)
}
