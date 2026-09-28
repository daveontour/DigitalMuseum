package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools wires every ChatBot-only utility tool. Deliberately no secrets, no filesystem
// access, no outbound network calls, no per-user data — pure computation over the call's own
// arguments. See main.go's package doc for why this server exists as a separate process.
func registerTools(server *mcp.Server) {
	server.AddTool(&mcp.Tool{
		Name:        "get_current_time",
		Description: "Get the current date and time in ISO format (UTC).",
		InputSchema: objSchema(nil, nil),
	}, wrapTool(func(_ map[string]any) (map[string]any, error) {
		now := time.Now().UTC()
		return map[string]any{"iso": now.Format(time.RFC3339), "unix": now.Unix()}, nil
	}))

	server.AddTool(&mcp.Tool{
		Name: "calculate",
		Description: "Evaluate a basic arithmetic expression (+, -, *, /, parentheses, decimals). " +
			"Use for quick calculations the user asks for in conversation.",
		InputSchema: objSchema(map[string]any{
			"expression": strProp("The arithmetic expression to evaluate, e.g. \"(3 + 4) * 2 / 7\""),
		}, []string{"expression"}),
	}, wrapTool(func(args map[string]any) (map[string]any, error) {
		expr, _ := args["expression"].(string)
		result, err := evalArithmetic(expr)
		if err != nil {
			return map[string]any{"error": err.Error()}, nil
		}
		return map[string]any{"expression": expr, "result": result}, nil
	}))

	server.AddTool(&mcp.Tool{
		Name: "convert_units",
		Description: "Convert a numeric value between common length, weight, or temperature units. " +
			"Supported length units: mm, cm, m, km, in, ft, yd, mi. Weight units: g, kg, oz, lb. " +
			"Temperature units: c, f, k.",
		InputSchema: objSchema(map[string]any{
			"value":     numProp("The numeric value to convert"),
			"from_unit": strProp("Source unit, e.g. \"km\", \"lb\", \"c\""),
			"to_unit":   strProp("Target unit, e.g. \"mi\", \"kg\", \"f\""),
		}, []string{"value", "from_unit", "to_unit"}),
	}, wrapTool(func(args map[string]any) (map[string]any, error) {
		value := toFloat(args["value"])
		from, _ := args["from_unit"].(string)
		to, _ := args["to_unit"].(string)
		result, err := convertUnits(value, from, to)
		if err != nil {
			return map[string]any{"error": err.Error()}, nil
		}
		return map[string]any{"value": value, "from_unit": from, "to_unit": to, "result": result}, nil
	}))

	server.AddTool(&mcp.Tool{
		Name:        "word_count",
		Description: "Count words, characters, and lines in a supplied piece of text.",
		InputSchema: objSchema(map[string]any{
			"text": strProp("The text to analyze"),
		}, []string{"text"}),
	}, wrapTool(func(args map[string]any) (map[string]any, error) {
		text, _ := args["text"].(string)
		return map[string]any{
			"words":      len(strings.Fields(text)),
			"characters": len([]rune(text)),
			"lines":      len(strings.Split(text, "\n")),
		}, nil
	}))

	server.AddTool(&mcp.Tool{
		Name:        "generate_uuid",
		Description: "Generate a new random UUID (v4).",
		InputSchema: objSchema(nil, nil),
	}, wrapTool(func(map[string]any) (map[string]any, error) {
		id, err := randomUUID()
		if err != nil {
			return map[string]any{"error": err.Error()}, nil
		}
		return map[string]any{"uuid": id}, nil
	}))

	server.AddTool(&mcp.Tool{
		Name:        "random_number",
		Description: "Generate a random integer in an inclusive [min, max] range.",
		InputSchema: objSchema(map[string]any{
			"min": intProp("Minimum value (inclusive)"),
			"max": intProp("Maximum value (inclusive)"),
		}, []string{"min", "max"}),
	}, wrapTool(func(args map[string]any) (map[string]any, error) {
		lo := toInt64(args["min"])
		hi := toInt64(args["max"])
		if hi < lo {
			return map[string]any{"error": "max must be >= min"}, nil
		}
		n, err := randomInt64InRange(lo, hi)
		if err != nil {
			return map[string]any{"error": err.Error()}, nil
		}
		return map[string]any{"result": n}, nil
	}))

	server.AddTool(&mcp.Tool{
		Name:        "base64_encode",
		Description: "Encode a piece of text as base64.",
		InputSchema: objSchema(map[string]any{
			"text": strProp("The text to encode"),
		}, []string{"text"}),
	}, wrapTool(func(args map[string]any) (map[string]any, error) {
		text, _ := args["text"].(string)
		return map[string]any{"result": base64.StdEncoding.EncodeToString([]byte(text))}, nil
	}))

	server.AddTool(&mcp.Tool{
		Name:        "base64_decode",
		Description: "Decode a base64-encoded string back to text.",
		InputSchema: objSchema(map[string]any{
			"text": strProp("The base64 string to decode"),
		}, []string{"text"}),
	}, wrapTool(func(args map[string]any) (map[string]any, error) {
		text, _ := args["text"].(string)
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("invalid base64: %v", err)}, nil
		}
		return map[string]any{"result": string(decoded)}, nil
	}))

	server.AddTool(&mcp.Tool{
		Name:        "text_case_convert",
		Description: "Convert text to upper case, lower case, or title case.",
		InputSchema: objSchema(map[string]any{
			"text": strProp("The text to convert"),
			"case": strProp("One of: \"upper\", \"lower\", \"title\""),
		}, []string{"text", "case"}),
	}, wrapTool(func(args map[string]any) (map[string]any, error) {
		text, _ := args["text"].(string)
		mode, _ := args["case"].(string)
		converted, err := convertTextCase(text, mode)
		if err != nil {
			return map[string]any{"error": err.Error()}, nil
		}
		return map[string]any{"result": converted}, nil
	}))
}

// ── schema helpers (mirrors cmd/mcpserver's, kept as a local copy — see main.go's package doc
// for why this binary avoids importing anything from the archive tools server) ──────────────

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func numProp(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}
func objSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

// wrapTool adapts a plain (args map[string]any) -> (result, error) function to the MCP SDK's
// ToolHandler shape. No caller-context reconstruction (unlike cmd/mcpserver's wrapTool) — these
// tools are not per-user, so there is nothing to reconstruct.
func wrapTool(fn func(args map[string]any) (map[string]any, error)) mcp.ToolHandler {
	return func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := map[string]any{}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
		}
		result, err := fn(args)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		return jsonResult(result), nil
	}
}

func jsonResult(v map[string]any) *mcp.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return errorResult(err.Error())
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	default:
		return 0
	}
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	default:
		return 0
	}
}

func randomUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// randomInt64InRange returns a uniformly random int64 in [lo, hi] using crypto/rand, avoiding
// modulo bias for the (small, user-supplied) ranges this tool expects.
func randomInt64InRange(lo, hi int64) (int64, error) {
	span := hi - lo + 1
	if span <= 0 {
		return 0, fmt.Errorf("invalid range")
	}
	// span fits comfortably in a uint64 for any realistic min/max the LLM would pass; reject
	// absurdly large ranges rather than risk overflow.
	if span > (1 << 62) {
		return 0, fmt.Errorf("range too large")
	}
	max := uint64(span)
	buf := make([]byte, 8)
	for {
		if _, err := rand.Read(buf); err != nil {
			return 0, err
		}
		v := uint64(0)
		for _, b := range buf {
			v = v<<8 | uint64(b)
		}
		// Reject values that would bias the modulo toward the low end.
		limit := (math.MaxUint64 / max) * max
		if v < limit {
			return lo + int64(v%max), nil
		}
	}
}

func convertTextCase(text, mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "upper":
		return strings.ToUpper(text), nil
	case "lower":
		return strings.ToLower(text), nil
	case "title":
		return toTitleCase(text), nil
	default:
		return "", fmt.Errorf(`case must be one of "upper", "lower", "title"`)
	}
}

func toTitleCase(s string) string {
	var b strings.Builder
	prevSpace := true
	for _, r := range s {
		if unicode.IsSpace(r) {
			prevSpace = true
			b.WriteRune(r)
			continue
		}
		if prevSpace {
			b.WriteRune(unicode.ToTitle(r))
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
		prevSpace = false
	}
	return b.String()
}
