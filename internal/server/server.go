// Package server exposes the subscription finder as MCP tools.
package server

import (
	"context"
	"errors"

	"github.com/GillMacedo/chargescout/internal/subs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FindInput is the input of subscriptions_find.
type FindInput struct {
	Path        string            `json:"path" jsonschema:"Path to the bank or card statement CSV"`
	Columns     map[string]string `json:"columns,omitempty" jsonschema:"Column names by field (date, amount, credit, debit, description), when detection fails"`
	DateFormat  string            `json:"date_format,omitempty" jsonschema:"dmy or mdy, needed only if the tool reports that dates are ambiguous"`
	PositiveOut bool              `json:"positive_out,omitempty" jsonschema:"Set if the file shows charges as positive numbers, as many card statements do"`
	Limit       *int              `json:"limit,omitempty" jsonschema:"Most findings to return, default 100. 0 returns only the totals"`
}

// FindOutput is the output of subscriptions_find.
type FindOutput struct {
	subs.Result
	FindingCount int   `json:"finding_count"`
	Truncated    bool  `json:"truncated"`
	SkippedRows  []int `json:"skipped_rows"`
}

// InspectInput is the input of subscriptions_inspect_csv.
type InspectInput struct {
	Path       string `json:"path" jsonschema:"Path to the statement CSV"`
	DateFormat string `json:"date_format,omitempty" jsonschema:"dmy or mdy"`
}

// InspectOutput holds no row contents.
type InspectOutput struct {
	Columns      map[string]string `json:"columns"`
	DateFormat   string            `json:"date_format"`
	Transactions int               `json:"transactions"`
	SkippedRows  int               `json:"skipped_rows"`
	Warnings     []string          `json:"warnings"`
}

func no() *bool { value := false; return &value }

// New returns the MCP server.
func New(version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "chargescout", Version: version}, nil)
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: no(), IdempotentHint: true, OpenWorldHint: no()}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "subscriptions_find",
		Title: "Find subscriptions and recurring charges",
		Description: "Finds subscriptions and other recurring charges in a bank or card statement CSV, with billing cycle, yearly cost, price increases and possible double charges. " +
			"Use to audit spending or find what to cancel. Read-only, and never uses the network. Results include merchant names and amounts from the file, so they reach the model. It can't cancel anything.",
		Annotations: annotations,
	}, find)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "subscriptions_inspect_csv",
		Title:       "Inspect a statement CSV",
		Description: "Shows which columns chargescout detects in a statement CSV and how many rows it can read, without returning row contents. Use if the file is unfamiliar or nothing was found.",
		Annotations: annotations,
	}, inspect)
	return server
}

func find(_ context.Context, _ *mcp.CallToolRequest, in FindInput) (*mcp.CallToolResult, FindOutput, error) {
	limit := 100
	if in.Limit != nil {
		limit = *in.Limit
	}
	if limit < 0 || limit > 500 {
		return nil, FindOutput{}, errors.New("limit must be between 0 and 500")
	}
	statement, err := subs.Load(in.Path, subs.Options{Columns: in.Columns, DateFormat: in.DateFormat, PositiveOut: in.PositiveOut})
	if err != nil {
		return nil, FindOutput{}, err
	}
	result := subs.Find(statement.Transactions)
	result.Warnings = append(result.Warnings, statement.Warnings...)
	out := FindOutput{Result: result, FindingCount: len(result.Findings), SkippedRows: statement.Skipped}
	if len(out.Findings) > limit {
		out.Findings, out.Truncated = out.Findings[:limit], true
	}
	return nil, out, nil
}

func inspect(_ context.Context, _ *mcp.CallToolRequest, in InspectInput) (*mcp.CallToolResult, InspectOutput, error) {
	statement, err := subs.Load(in.Path, subs.Options{DateFormat: in.DateFormat})
	if err != nil {
		return nil, InspectOutput{}, err
	}
	return nil, InspectOutput{Columns: statement.Columns, DateFormat: statement.DateFormat, Transactions: len(statement.Transactions), SkippedRows: len(statement.Skipped), Warnings: statement.Warnings}, nil
}
