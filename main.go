// Command chargescout finds subscriptions and recurring charges in bank statements.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GillMacedo/chargescout/internal/server"
	"github.com/GillMacedo/chargescout/internal/subs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "0.2.0"

const usage = `Usage:
  chargescout find <statement.csv>   Find subscriptions and recurring charges
  chargescout serve                  Run as an MCP server over stdio
  chargescout version

Options for find:
  --positive-out                  The file shows charges as positive numbers
  --date-format dmy|mdy           Needed only when dates are ambiguous
  --columns date=When,amount=Sum  Name columns the tool doesn't recognize
  --json                          Print JSON

Reads your file and never changes it. Never uses the network.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return 2
	}
	switch args[0] {
	case "serve":
		if err := server.New(version).Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			fmt.Fprintf(stderr, "chargescout: %v\n", err)
			return 2
		}
		return 0
	case "version":
		fmt.Fprintln(stdout, version)
		return 0
	case "find":
		if err := find(args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "chargescout: %v\n", err)
			return 2
		}
		return 0
	}
	fmt.Fprint(stdout, usage)
	return 2
}

func find(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("find", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	columnsFlag := flags.String("columns", "", "")
	dateFormat := flags.String("date-format", "", "")
	positiveOut := flags.Bool("positive-out", false, "")
	asJSON := flags.Bool("json", false, "")

	// The standard flag package stops at the first path, so options can't
	// follow it without this loop.
	var paths []string
	for rest := args; ; {
		if err := flags.Parse(rest); err != nil {
			return err
		}
		if flags.NArg() == 0 {
			break
		}
		paths = append(paths, flags.Arg(0))
		rest = flags.Args()[1:]
	}
	if len(paths) != 1 {
		return fmt.Errorf("find takes one CSV file\n%s", usage)
	}
	if *dateFormat != "" && *dateFormat != "dmy" && *dateFormat != "mdy" {
		return fmt.Errorf("--date-format must be dmy or mdy")
	}
	overrides := map[string]string{}
	if *columnsFlag != "" {
		for _, pair := range strings.Split(*columnsFlag, ",") {
			field, name, ok := strings.Cut(pair, "=")
			if !ok {
				return fmt.Errorf("--columns needs field=Column pairs, not %q", pair)
			}
			overrides[strings.TrimSpace(field)] = strings.TrimSpace(name)
		}
	}

	statement, err := subs.Load(paths[0], subs.Options{Columns: overrides, DateFormat: *dateFormat, PositiveOut: *positiveOut})
	if err != nil {
		return err
	}
	result := subs.Find(statement.Transactions)
	result.Warnings = append(result.Warnings, statement.Warnings...)

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(struct {
			subs.Result
			SkippedRows []int `json:"skipped_rows"`
		}{result, statement.Skipped})
	}
	for _, finding := range result.Findings {
		line := fmt.Sprintf("%s  %s  %s  %s/year  last %s  %s", finding.Merchant, finding.LastAmount, finding.Cadence, finding.YearlyCost, finding.LastDate, finding.Status)
		if finding.Confidence == subs.Likely {
			line += "  (likely)"
		}
		fmt.Fprintln(stdout, line)
		for _, alert := range finding.Alerts {
			fmt.Fprintf(stdout, "  ! %s\n", alert)
		}
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(stdout, "WARNING: %s\n", warning)
	}
	for _, row := range statement.Skipped {
		fmt.Fprintf(stdout, "SKIPPED row %d: no valid date or amount\n", row)
	}
	fmt.Fprintf(stdout, "%d recurring charges, %d active, costing %s a year\n", len(result.Findings), result.ActiveCount, result.ActiveYearly)
	return nil
}
