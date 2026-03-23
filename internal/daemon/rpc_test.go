package daemon_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/anutron/claude-command-center/internal/daemon"
)

func TestEncodeDecodeRequest(t *testing.T) {
	req := daemon.RPCRequest{
		Method: "RegisterSession",
		ID:     1,
		Params: json.RawMessage(`{"session_id":"abc"}`),
	}
	var buf bytes.Buffer
	err := daemon.WriteMessage(&buf, req)
	if err != nil {
		t.Fatal(err)
	}
	var decoded daemon.RPCRequest
	err = daemon.ReadMessage(&buf, &decoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Method != "RegisterSession" {
		t.Fatalf("expected RegisterSession, got %s", decoded.Method)
	}
}

func TestEncodeDecodeResponse(t *testing.T) {
	resp := daemon.RPCResponse{
		ID:     1,
		Result: json.RawMessage(`{"ok":true}`),
	}
	var buf bytes.Buffer
	daemon.WriteMessage(&buf, resp)
	var decoded daemon.RPCResponse
	daemon.ReadMessage(&buf, &decoded)
	if decoded.ID != 1 {
		t.Fatalf("expected ID 1, got %d", decoded.ID)
	}
}
