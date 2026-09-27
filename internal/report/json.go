package report

import (
	"encoding/json"
	"io"

	"ai-dev/internal/doctor"
)

type jsonReport struct {
	Overall string      `json:"overall"`
	Checks  []jsonCheck `json:"checks"`
}

type jsonCheck struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Area        string `json:"area"`
	Status      string `json:"status"`
	Explanation string `json:"explanation"`
	Remediation string `json:"remediation"`
}

func WriteJSON(w io.Writer, report doctor.Report) error {
	encoded := jsonReport{Overall: "ready", Checks: make([]jsonCheck, 0, len(report.Checks))}
	if report.Overall == doctor.Fail {
		encoded.Overall = "not_ready"
	}
	for _, result := range report.Checks {
		status := "pass"
		if result.Status == doctor.Fail {
			status = "fail"
		}
		encoded.Checks = append(encoded.Checks, jsonCheck{
			ID:          result.ID,
			Name:        result.Name,
			Area:        result.Area,
			Status:      status,
			Explanation: result.Explanation,
			Remediation: result.Remediation,
		})
	}
	return json.NewEncoder(w).Encode(encoded)
}
