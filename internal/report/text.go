// Package report renders readiness reports for humans and automation.
package report

import (
	"fmt"
	"io"

	"ai-dev/internal/doctor"
)

func WriteText(w io.Writer, report doctor.Report) error {
	area := ""
	for _, result := range report.Checks {
		if result.Area != area {
			if area != "" {
				if _, err := fmt.Fprintln(w); err != nil {
					return err
				}
			}
			area = result.Area
			if _, err := fmt.Fprintln(w, area); err != nil {
				return err
			}
		}
		status := "PASS"
		if result.Status == doctor.Fail {
			status = "FAIL"
		}
		if _, err := fmt.Fprintf(w, "  [%s] %s\n", status, result.Name); err != nil {
			return err
		}
		if result.Status == doctor.Fail {
			if _, err := fmt.Fprintf(w, "        Explanation: %s\n        Remediation: %s\n", result.Explanation, result.Remediation); err != nil {
				return err
			}
		}
	}
	overall := "Ready"
	if report.Overall == doctor.Fail {
		overall = "Not Ready"
	}
	if _, err := fmt.Fprintf(w, "\nOverall: %s\n", overall); err != nil {
		return err
	}
	return nil
}
