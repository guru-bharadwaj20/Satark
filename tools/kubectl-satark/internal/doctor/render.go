package doctor

import (
	"fmt"
	"io"
	"strings"
)

var icons = map[Severity]string{OK: "[ OK ]", Warn: "[WARN]", Fail: "[FAIL]"}

// RenderText writes a human-readable report.
func RenderText(w io.Writer, results []Result, verbose bool) {
	for _, r := range results {
		fmt.Fprintf(w, "%s %-12s %s\n", icons[r.Status], r.Check, r.Summary)
		if r.Error != "" {
			fmt.Fprintf(w, "       error: %s\n", r.Error)
		}
		for _, f := range r.Findings {
			if f.Severity == OK && !verbose {
				continue
			}
			loc := f.Object
			if f.Namespace != "" {
				loc = f.Namespace + "/" + f.Object
			}
			fmt.Fprintf(w, "       %-4s %s: %s\n", strings.ToUpper(string(f.Severity)), loc, f.Message)
		}
	}
	fmt.Fprintf(w, "\nOverall: %s\n", strings.ToUpper(string(Worst(results))))
}
