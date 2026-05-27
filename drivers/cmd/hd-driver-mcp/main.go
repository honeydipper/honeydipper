// Package main provides a remote MCP (Model Context Protocol) client driver for Honeydipper.
//
// It connects to remote MCP servers over SSE or Streamable HTTP transport and exposes
// two RPC calls consumed by the agent service:
//
//   - list_tools   – given a server name, returns the server's tool list.
//   - call_tool    – given a server name, tool name and arguments, calls the tool.
//
// Server configuration lives in the driver options under data.servers.<name>:
//
//	data:
//	  servers:
//	    my-server:
//	      url: https://example.com/mcp
//	      transport: streamable   # or "sse"; default "streamable"
//	      headers:
//	        Authorization: "Bearer <token>"
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverConfig holds the configuration for a single remote MCP server.
type serverConfig struct {
	URL       string            `mapstructure:"url"`
	Transport string            `mapstructure:"transport"` // "streamable" (default) or "sse"
	Headers   map[string]string `mapstructure:"headers"`
}

var driver *dipper.Driver

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s [ -h ] <service name>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    This driver supports the operator service.\n")
		fmt.Fprintf(os.Stderr, "    It provides Honeydipper agents with access to remote MCP servers.\n")
	}
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "mcp")
	driver.RPCHandlers["list_tools"] = listTools
	driver.RPCHandlers["call_tool"] = callTool
	driver.Reload = func(_ *dipper.Message) {}
	driver.Run()
}

// getServerConfig reads and decodes a named server's configuration from driver options.
func getServerConfig(name string) serverConfig {
	raw, ok := dipper.GetMapData(driver.Options, "data.servers."+name)
	if !ok || raw == nil {
		dipper.Logger.Panicf("[mcp] unknown server %q", name)
	}

	var cfg serverConfig
	if err := mapstructure.Decode(raw, &cfg); err != nil {
		dipper.Logger.Panicf("[mcp] invalid config for server %q: %v", name, err)
	}

	return cfg
}

// headerRoundTripper injects static HTTP headers into every outbound request.
type headerRoundTripper struct {
	headers map[string]string
	wrapped http.RoundTripper
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid mutating the original.
	r2 := req.Clone(req.Context())
	for k, v := range t.headers {
		r2.Header.Set(k, v)
	}

	resp, err := t.wrapped.RoundTrip(r2)
	if err != nil {
		return nil, fmt.Errorf("mcp header transport: %w", err)
	}

	return resp, nil
}

// buildHTTPClient returns a custom *http.Client that injects cfg.Headers, or nil
// when no headers are configured (the SDK then uses its own default client).
func buildHTTPClient(headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return nil
	}

	return &http.Client{
		Transport: &headerRoundTripper{
			headers: headers,
			wrapped: http.DefaultTransport,
		},
	}
}

// connectToServer creates an MCP client session connected to the given server.
// The caller is responsible for calling session.Close() when done.
func connectToServer(ctx context.Context, name string, cfg serverConfig) *mcp.ClientSession {
	client := mcp.NewClient(&mcp.Implementation{Name: "honeydipper", Version: "1.0"}, nil)
	httpClient := buildHTTPClient(cfg.Headers)

	var transport mcp.Transport
	switch cfg.Transport {
	case "sse":
		transport = mcp.NewSSEClientTransport(cfg.URL, &mcp.SSEClientTransportOptions{
			HTTPClient: httpClient,
		})
	default: // "streamable" or empty
		transport = mcp.NewStreamableClientTransport(cfg.URL, &mcp.StreamableClientTransportOptions{
			HTTPClient: httpClient,
		})
	}

	session, err := client.Connect(ctx, transport)
	if err != nil {
		dipper.Logger.Panicf("[mcp] failed to connect to server %q: %v", name, err)
	}

	return session
}

// listTools handles the list_tools RPC call.
//
// Expected payload fields:
//   - server (string): name of the server entry in data.servers
//
// Response payload:
//
//	{"tools": [{"name": "...", "description": "...", "input_schema": {...}}, ...]}
func listTools(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	serverName := dipper.MustGetMapDataStr(msg.Payload, "server")
	cfg := getServerConfig(serverName)

	ctx := context.Background()

	session := connectToServer(ctx, serverName, cfg)
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		dipper.Logger.Panicf("[mcp] list_tools failed server=%s: %v", serverName, err)
	}

	tools := make([]map[string]interface{}, 0, len(result.Tools))
	for _, t := range result.Tools {
		tool := map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
		}

		if t.InputSchema != nil {
			schemaBytes, err := json.Marshal(t.InputSchema)
			if err == nil {
				var schema map[string]interface{}
				if json.Unmarshal(schemaBytes, &schema) == nil {
					tool["input_schema"] = schema
				}
			}
		}

		tools = append(tools, tool)
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"tools": tools,
		},
	}
}

// callTool handles the call_tool RPC call.
//
// Expected payload fields:
//   - server (string): name of the server entry in data.servers
//   - tool   (string): name of the MCP tool to call
//   - args   (map):    arguments to pass to the tool (may be nil/absent)
//
// Response payload:
//
//	{"output": "<text content>", "is_error": <bool>}
func callTool(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	serverName := dipper.MustGetMapDataStr(msg.Payload, "server")
	toolName := dipper.MustGetMapDataStr(msg.Payload, "tool")
	cfg := getServerConfig(serverName)

	var argsMap map[string]interface{}
	if raw, ok := dipper.GetMapData(msg.Payload, "args"); ok && raw != nil {
		if err := mapstructure.Decode(raw, &argsMap); err != nil {
			dipper.Logger.Warningf("[mcp] failed to decode args for tool %s: %v", toolName, err)
		}
	}

	ctx := context.Background()

	session := connectToServer(ctx, serverName, cfg)
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: argsMap,
	})
	if err != nil {
		dipper.Logger.Panicf("[mcp] call_tool failed server=%s tool=%s: %v", serverName, toolName, err)
	}

	var sb strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"output":   sb.String(),
			"is_error": result.IsError,
		},
	}
}
