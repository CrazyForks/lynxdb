package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lynxbase/lynxdb/internal/ui"
)

func init() {
	rootCmd.AddCommand(newExamplesCmd())
}

func newExamplesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "examples",
		Aliases: []string{"cookbook"},
		Short:   "Show LynxFlow query examples",
		Long:    `A cookbook of common LynxFlow query patterns for log analysis.`,
		RunE:    runExamples,
	}
}

func runExamples(_ *cobra.Command, _ []string) error {
	t := ui.Stdout

	fmt.Printf("\n  %s\n\n", t.Bold.Render("LynxFlow Query Cookbook"))

	sections := []struct {
		title    string
		examples []struct{ query, desc string }
	}{
		{
			title: "Search & Filter",
			examples: []struct{ query, desc string }{
				{`from main | where level == "error"`, "Find all error events"},
				{`from main | where level == "error" and source == "nginx"`, "Errors from nginx"},
				{`from main | where status >= 500`, "Server errors by status code"},
				{`from main | where has(_raw, "connection refused")`, "Full-text search"},
				{`from main | where duration_ms > 1000`, "Slow requests"},
			},
		},
		{
			title: "Aggregation",
			examples: []struct{ query, desc string }{
				{`from main | where level == "error" | stats count()`, "Count errors"},
				{`from main | where level == "error" | stats count() by source`, "Errors per source"},
				{`from main | stats avg(duration_ms), perc99(duration_ms) by endpoint`, "Latency stats"},
				{`from main | where status >= 500 | top 10 uri`, "Top failing URIs"},
				{`from main | stats dc(user_id) as unique_users`, "Distinct users"},
			},
		},
		{
			title: "Time Analysis",
			examples: []struct{ query, desc string }{
				{`from main | where level == "error" | every 5m stats count()`, "Error rate over time"},
				{`from main | every 1h by service stats avg(duration_ms)`, "Latency by service"},
				{`from main | every 1h by level stats count()`, "Hourly breakdown"},
			},
		},
		{
			title: "Transformation",
			examples: []struct{ query, desc string }{
				{`from main | extend is_slow = if(duration_ms > 1000, "yes", "no")`, "Computed field"},
				{`from main | parse regex r"user=(?P<user>\w+)"`, "Extract with regex"},
				{`from main | rename status as http_status`, "Rename fields"},
				{`from main | keep _time, source, level, message`, "Select columns"},
			},
		},
		{
			title: "Local File Queries",
			examples: []struct{ query, desc string }{
				{`lynxdb query --file app.log '| stats count() by level'`, "Query local file"},
				{`cat app.json | lynxdb query '| where level == "ERROR"'`, "Pipe from stdin"},
				{`lynxdb query --file '*.log' '| stats count() by source'`, "Glob pattern"},
			},
		},
	}

	for _, s := range sections {
		fmt.Printf("  %s\n", t.Bold.Render(s.title))
		for _, ex := range s.examples {
			fmt.Printf("    %s %s\n", t.Info.Render(fmt.Sprintf("%-60s", ex.query)), t.Dim.Render(ex.desc))
		}
		fmt.Println()
	}

	printHint("Run 'lynxdb query --help' for all query options.")
	fmt.Println()

	return nil
}
