package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mcpSession runs the MCP server over pipes and returns the responses.
func mcpSession(t *testing.T, esp string, requests ...string) []map[string]any {
	t.Helper()

	vars := filepath.Join(t.TempDir(), "efivars")
	if err := os.MkdirAll(vars, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SDBCTL_EFIVARS", vars)

	in := strings.NewReader(strings.Join(requests, "\n") + "\n")
	var out, errb bytes.Buffer

	g := &globals{esp: esp}
	if code := serveMCP(in, &out, &errb, g); code != ExitOK {
		t.Fatalf("mcp server exited with %d: %s", code, errb.String())
	}

	var responses []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("response is not JSON: %v\n%s", err, line)
		}
		responses = append(responses, resp)
	}
	return responses
}

func TestMCPInitializeAndList(t *testing.T) {
	esp := fakeESP(t)

	resp := mcpSession(t, esp,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	)
	if len(resp) != 2 {
		t.Fatalf("expected 2 responses (the notification must not be answered), got %d: %v", len(resp), resp)
	}

	init, _ := resp[0]["result"].(map[string]any)
	if init["protocolVersion"] == nil {
		t.Errorf("initialize result missing protocolVersion: %v", init)
	}
	if _, ok := init["capabilities"].(map[string]any)["tools"]; !ok {
		t.Errorf("server does not advertise the tools capability: %v", init)
	}
	if srv, _ := init["serverInfo"].(map[string]any); srv["name"] != "sdbctl" {
		t.Errorf("serverInfo wrong: %v", init)
	}

	listed, _ := resp[1]["result"].(map[string]any)
	tools, _ := listed["tools"].([]any)
	names := map[string]bool{}
	for _, tool := range tools {
		m, _ := tool.(map[string]any)
		name, _ := m["name"].(string)
		names[name] = true
		if m["description"] == nil || m["inputSchema"] == nil {
			t.Errorf("tool %s is missing description or inputSchema", name)
		}
	}
	if !names["sdbctl_status"] || !names["sdbctl_list_entries"] {
		t.Errorf("read-only tools missing: %v", names)
	}
	// The server is read-only: no tool that changes anything may exist.
	for _, forbidden := range []string{
		"sdbctl_boot", "sdbctl_set_default", "sdbctl_set_timeout",
		"sdbctl_set_key", "sdbctl_remove_entry", "sdbctl_undo",
	} {
		if names[forbidden] {
			t.Errorf("%s must not be advertised: the MCP server is read-only", forbidden)
		}
	}
	if len(names) != 5 {
		t.Errorf("expected exactly the five read-only tools, got %v", names)
	}
	if !names["sdbctl_recommend"] {
		t.Errorf("the recommender tool should be offered: %v", names)
	}
}

func TestMCPStatusCall(t *testing.T) {
	esp := fakeESP(t)

	resp := mcpSession(t, esp,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sdbctl_status","arguments":{}}}`,
	)
	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}
	result, _ := resp[0]["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("status call reported an error: %v", result)
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("no content returned: %v", result)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	if !strings.Contains(text, "Arch Linux") {
		t.Errorf("status text missing entries:\n%s", text)
	}
	if result["structuredContent"] == nil {
		t.Error("structured content missing — clients should be able to use the JSON")
	}
}

func TestMCPHasNoWriteTools(t *testing.T) {
	esp := fakeESP(t)

	// Calling a mutating tool must be refused: the server does not even
	// know it, so nothing can be changed through it.
	resp := mcpSession(t, esp,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sdbctl_boot","arguments":{"entry":"fedora"}}}`,
	)
	rpcErr, ok := resp[0]["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected a JSON-RPC error for a write tool, got %v", resp[0])
	}
	if code, _ := rpcErr["code"].(float64); code != -32601 && code != -32602 {
		t.Errorf("unexpected error code %v: %v", code, rpcErr)
	}

	// And the machine must be untouched.
	if got, _ := oneShot(t); got != "" {
		t.Errorf("a one-shot was written through the MCP server: %q", got)
	}
}

func TestMCPUnknownMethodAndTool(t *testing.T) {
	esp := fakeESP(t)

	resp := mcpSession(t, esp,
		`{"jsonrpc":"2.0","id":1,"method":"nonsense/method"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"sdbctl_nope","arguments":{}}}`,
	)
	if len(resp) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resp))
	}
	for _, r := range resp {
		errObj, ok := r["error"].(map[string]any)
		if !ok {
			t.Errorf("expected a JSON-RPC error, got %v", r)
			continue
		}
		if code, _ := errObj["code"].(float64); code != -32601 && code != -32602 {
			t.Errorf("unexpected error code %v: %v", code, errObj)
		}
	}
}
