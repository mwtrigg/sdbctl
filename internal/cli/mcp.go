package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// sdbctl speaks MCP (Model Context Protocol) over stdio, so agents can
// *inspect* a machine's boot setup with the same semantics the CLI has. The
// server is a thin wrapper over API: there is no second implementation of
// any behaviour to drift out of sync.
//
// The server is read-only, and that is not a temporary restriction: there is
// no --allow-write, and no tool that changes anything. An agent can answer
// "what will this machine boot?", "which boot options do I have?", "is this
// entry going to fail?" and "what could I roll back to?" — and nothing
// else. Changing boot settings stays a deliberate act by a human at a
// terminal, because a stray tool call on the wrong machine can make it
// unbootable.
//
// The protocol is JSON-RPC 2.0 with newline-delimited messages.

const mcpProtocolVersion = "2025-06-18"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	annotations map[string]any //nolint:unused // reserved for MCP tool annotations
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content        []toolContent `json:"content"`
	IsError        bool          `json:"isError,omitempty"`
	StructuredData any           `json:"structuredContent,omitempty"`
}

// mcpServer holds the connection state for one client.
type mcpServer struct {
	api    *API
	stderr io.Writer
}

func (s *mcpServer) tools() []toolDef {
	all := []toolDef{
		{
			Name:        "sdbctl_status",
			Description: "Boot status for the target machine: which entry boots by default, what is set to boot once, and entries systemd-boot will hide.",
			InputSchema: objectSchema(map[string]any{}, nil),
		},
		{
			Name:        "sdbctl_list_entries",
			Description: "List boot loader entries with their identifiers, image sources, and issues.",
			InputSchema: objectSchema(map[string]any{}, nil),
		},
		{
			Name:        "sdbctl_check",
			Description: "Full diagnostics: boot loader binaries and versions, loader.conf problems, and per-entry reasons an entry is hidden from the boot menu.",
			InputSchema: objectSchema(map[string]any{}, nil),
		},
		{
			Name:        "sdbctl_snapshots",
			Description: "List snapshots taken before changes made by this tool.",
			InputSchema: objectSchema(map[string]any{}, nil),
		},
		{
			Name: "sdbctl_recommend",
			Description: "Given a goal in plain language (\"boot Fedora once\", \"make this the default\", " +
				"\"why is my entry hidden\"), return the sdbctl commands to run and what each would change. " +
				"It only recommends: the server never changes anything.",
			InputSchema: objectSchema(map[string]any{
				"goal":  stringProp("what you want to achieve, in plain language"),
				"entry": stringProp("optional: the entry id the goal refers to"),
			}, []string{"goal"}),
		},
	}

	return all
}

func objectSchema(props map[string]any, required []string) map[string]any {
	s := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func stringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

// serveMCP runs the stdio server until stdin closes. It never changes
// anything: the server is read-only by design.
func serveMCP(stdin io.Reader, stdout, stderr io.Writer, g *globals) int {
	api, err := Resolve(g.esp)
	if err != nil {
		fmt.Fprintf(stderr, "sdbctl mcp: %v\n", err)
		return ExitFailure
	}
	api.DryRun = g.dryRun
	api.AllowHidden = g.force

	s := &mcpServer{api: api, stderr: stderr}
	fmt.Fprintln(stderr, "sdbctl mcp: read-only — this server cannot change any setting")

	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	out := bufio.NewWriter(stdout)
	defer out.Flush()

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeRPC(out, nil, &rpcError{Code: -32700, Message: "parse error"})
			continue
		}
		// Notifications carry no id and must not be answered.
		isNotification := len(req.ID) == 0 || string(req.ID) == "null"
		result, rerr := s.handle(req)
		if isNotification {
			continue
		}
		writeRPC(out, result, rerr, req.ID)
		out.Flush()
	}
	if err := sc.Err(); err != nil && err != io.EOF {
		fmt.Fprintf(stderr, "sdbctl mcp: %v\n", err)
		return ExitFailure
	}
	return ExitOK
}

func writeRPC(w io.Writer, result any, e *rpcError, id ...json.RawMessage) {
	resp := rpcResponse{JSONRPC: "2.0", Result: result, Error: e}
	if len(id) > 0 {
		resp.ID = id[0]
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	w.Write(append(data, '\n'))
}

func (s *mcpServer) handle(req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "sdbctl", "version": Version},
		}, nil

	case "notifications/initialized", "notifications/cancelled":
		return nil, nil

	case "ping":
		return map[string]any{}, nil

	case "tools/list":
		return map[string]any{"tools": s.tools()}, nil

	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid tool arguments"}
		}
		return s.call(p.Name, p.Arguments)

	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
}

func (s *mcpServer) call(name string, raw json.RawMessage) (any, *rpcError) {
	var args map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid arguments: " + err.Error()}
		}
	}
	str := func(k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	switch name {
	case "sdbctl_status", "sdbctl_check":
		st, err := s.api.Status()
		return s.marshal(st, err)

	case "sdbctl_list_entries":
		entries, err := s.api.List()
		return s.marshal(map[string]any{"entries": entries}, err)

	case "sdbctl_snapshots":
		snaps, err := s.api.Snapshots()
		return s.marshal(map[string]any{"snapshots": snaps}, err)

	case "sdbctl_recommend":
		recs, err := s.api.Recommend(str("goal"), str("entry"))
		return s.marshal(map[string]any{"recommendations": recs}, err)

	}

	return nil, &rpcError{Code: -32602, Message: "unknown tool: " + name}
}

// marshal renders one result as both human text (content) and machine data
// (structuredContent), so a client can use either.
func (s *mcpServer) marshal(v any, err error) (any, *rpcError) {
	if err != nil {
		return toolResult{
			Content: []toolContent{{Type: "text", Text: "sdbctl: " + err.Error()}},
			IsError: true,
		}, nil
	}

	var text string
	switch x := v.(type) {
	case *Status:
		var b strings.Builder
		writeStatus(&b, x)
		text = b.String()
	default:
		data, merr := json.MarshalIndent(v, "", "  ")
		if merr != nil {
			return nil, &rpcError{Code: -32603, Message: merr.Error()}
		}
		text = string(data)
	}

	return toolResult{
		Content:        []toolContent{{Type: "text", Text: text}},
		StructuredData: v,
	}, nil
}
