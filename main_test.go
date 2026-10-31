package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/GillMacedo/chargescout/internal/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const sample = "examples/statement.csv"

func TestFindPrints(t *testing.T) {
	var out, errs bytes.Buffer
	if code := run([]string{"find", sample}, &out, &errs); code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	for _, want := range []string{"SAMPLE STREAM", "SAMPLE POWER", "usage-based", "price went up from 15.99 to 18.99", "possible double charge", "FICTIONAL GYM", "may have stopped", "SKIPPED row"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "DEMO BAKERY") || strings.Contains(out.String(), "CUSTOMER PAYMENT") {
		t.Errorf("non-recurring spending or income listed:\n%s", out.String())
	}
}

func TestOptionsCanFollowThePath(t *testing.T) {
	var out, errs bytes.Buffer
	if code := run([]string{"find", sample, "--json"}, &out, &errs); code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body["findings"].([]any)) != 6 {
		t.Errorf("got %v", body["findings"])
	}
}

func TestErrorsExitTwo(t *testing.T) {
	var out, errs bytes.Buffer
	for _, args := range [][]string{{"find"}, {"find", "/no/such.csv"}, {"nope"}, {}, {"find", sample, "--columns", "oops"}, {"find", sample, "--date-format", "x"}} {
		if code := run(args, &out, &errs); code != 2 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}

func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	clientSide, serverSide := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.New("test").Connect(ctx, serverSide, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func call(t *testing.T, name string, args map[string]any) (map[string]any, *mcp.CallToolResult) {
	t.Helper()
	result, err := connect(t).CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{}
	if !result.IsError {
		data, _ := json.Marshal(result.StructuredContent)
		json.Unmarshal(data, &body)
	}
	return body, result
}

func TestServerListsReadOnlyTools(t *testing.T) {
	list, err := connect(t).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 2 {
		t.Fatalf("got %d tools", len(list.Tools))
	}
	for _, tool := range list.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s isn't read-only", tool.Name)
		}
	}
}

func TestServerFinds(t *testing.T) {
	body, _ := call(t, "subscriptions_find", map[string]any{"path": sample})
	if body["finding_count"] != float64(6) {
		t.Errorf("got %v", body)
	}
	body, _ = call(t, "subscriptions_find", map[string]any{"path": sample, "limit": 0})
	if body["truncated"] != true || len(body["findings"].([]any)) != 0 || body["finding_count"] != float64(6) {
		t.Errorf("got %v", body)
	}
}

func TestServerInspectHasNoRowContents(t *testing.T) {
	body, result := call(t, "subscriptions_inspect_csv", map[string]any{"path": sample})
	if body["transactions"].(float64) < 40 {
		t.Errorf("got %v", body)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "SAMPLE STREAM") {
		t.Error("inspect returned row contents")
	}
}

func TestServerBadPathIsAToolError(t *testing.T) {
	_, result := call(t, "subscriptions_find", map[string]any{"path": "/no/such.csv"})
	if !result.IsError {
		t.Error("want a tool error")
	}
}

func TestSourceHasNoNetworkCode(t *testing.T) {
	for _, file := range []string{"main.go", "internal/subs/load.go", "internal/subs/find.go", "internal/server/server.go"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{`"net/http"`, `"net"`} {
			if strings.Contains(string(data), banned) {
				t.Errorf("%s imports %s", file, banned)
			}
		}
	}
}
