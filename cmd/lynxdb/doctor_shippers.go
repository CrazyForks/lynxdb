package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lynxbase/lynxdb/pkg/client"
)

type shipperDoctorReport struct {
	Listeners []shipperListenerCheck      `json:"listeners"`
	Recent    []client.ShipperObservation `json:"recent"`
	Warnings  []string                    `json:"warnings,omitempty"`
}

type shipperListenerCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func newDoctorShippersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shippers",
		Short: "Diagnose log shipper compatibility",
		RunE:  runDoctorShippers,
	}
}

func runDoctorShippers(_ *cobra.Command, _ []string) error {
	ctx := context.Background()
	c := apiClient()

	listeners, err := fetchShipperListenerChecks(ctx)
	if err != nil {
		return err
	}
	recent, err := c.Shippers(ctx)
	if err != nil {
		return err
	}
	report := buildShipperDoctorReport(listeners, recent)

	if isJSONFormat() {
		b, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	printShipperDoctorReport(report)
	return nil
}

func buildShipperDoctorReport(listeners []shipperListenerCheck, recent []client.ShipperObservation) shipperDoctorReport {
	report := shipperDoctorReport{Listeners: listeners, Recent: recent}
	if len(recent) == 0 {
		report.Warnings = append(report.Warnings, "no shipper traffic observed yet")
		return report
	}

	var sawHEC bool
	for _, s := range recent {
		if s.Tool == "splunk-hec" || strings.HasPrefix(s.Endpoint, "/services/collector") {
			sawHEC = true
			break
		}
	}
	if !sawHEC {
		report.Warnings = append(report.Warnings, "Splunk HEC has not been exercised recently")
	}
	return report
}

func printShipperDoctorReport(report shipperDoctorReport) {
	fmt.Fprintln(os.Stdout, "LynxDB doctor - shipper diagnostics")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Listener health:")
	for _, l := range report.Listeners {
		fmt.Fprintf(os.Stdout, "  %-10s %s\n", l.Name, l.Status)
	}

	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Recent ingest:")
	if len(report.Recent) == 0 {
		fmt.Fprintln(os.Stdout, "  none")
	} else {
		for _, s := range report.Recent {
			fmt.Fprintf(os.Stdout, "  %-12s %-8s %-8s %-8s %s\n",
				shipperNameVersion(s),
				s.Status,
				formatShipperLastSeen(s.LastSeenAt),
				formatCountHuman(s.EventsPerMin)+"/min",
				s.Endpoint,
			)
		}
	}

	if len(report.Warnings) > 0 {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "Diagnostics:")
		for _, w := range report.Warnings {
			fmt.Fprintf(os.Stdout, "  WARN %s\n", w)
		}
	}
}

func fetchShipperListenerChecks(ctx context.Context) ([]shipperListenerCheck, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(globalServer, "/")+"/metrics", nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("metrics request failed: status %d", resp.StatusCode)
	}

	values := map[string]float64{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "lynxdb_ingest_listener_up{") {
			continue
		}
		listener := metricLabel(line, "listener")
		fields := strings.Fields(line)
		if listener == "" || len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err == nil {
			values[listener] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return []shipperListenerCheck{
		{Name: "ES bulk", Status: boundStatus(values["es"] > 0)},
		{Name: "OTLP HTTP", Status: boundStatus(values["otlp_http"] > 0)},
		{Name: "OTLP gRPC", Status: boundStatus(values["otlp_grpc"] > 0)},
	}, nil
}

func metricLabel(line, name string) string {
	needle := name + `="`
	start := strings.Index(line, needle)
	if start < 0 {
		return ""
	}
	start += len(needle)
	end := strings.IndexByte(line[start:], '"')
	if end < 0 {
		return ""
	}
	return line[start : start+end]
}

func boundStatus(bound bool) string {
	if bound {
		return "bound"
	}
	return "not bound"
}

func shipperNameVersion(s client.ShipperObservation) string {
	if s.Version == "" {
		return s.Tool
	}
	return s.Tool + "/" + s.Version
}
