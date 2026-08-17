package timeline

import (
	"fmt"
	"strings"
	"time"
)

// RenderMarkdown formats a timeline as a Markdown report suitable for pasting
// straight into an incident document. It emits a heading naming the incident
// window, followed by a table of every event with its timestamp, source and
// description. Each anomaly that carries a Suspect annotation surfaces its
// nearest preceding change on a second line inside its own row.
func RenderMarkdown(events []Event, since, until time.Time) string {
	var b strings.Builder

	b.WriteString("# Incident timeline\n\n")
	fmt.Fprintf(&b, "**Window:** %s → %s\n\n",
		since.Format(time.RFC3339), until.Format(time.RFC3339))

	if len(events) == 0 {
		b.WriteString("_No events in this window._\n")
		return b.String()
	}

	b.WriteString("| Time | Source | Event |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, e := range events {
		event := escapeCell(e.Summary)
		if e.Suspect != "" {
			event += "<br>**suspect:** " + escapeCell(e.Suspect)
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n",
			e.Time.Format(time.RFC3339), escapeCell(e.Source), event)
	}

	return b.String()
}

// escapeCell makes a value safe to drop into a Markdown table cell: pipes are
// escaped so they do not start a new column, and any embedded newline is
// flattened to a space so a value stays on its own row.
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}
