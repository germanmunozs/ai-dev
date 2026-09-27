package report

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"ai-dev/internal/doctor"
)

type decodedReport struct {
	Overall string         `json:"overall"`
	Checks  []decodedCheck `json:"checks"`
}

type decodedCheck struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Area        string `json:"area"`
	Status      string `json:"status"`
	Explanation string `json:"explanation"`
	Remediation string `json:"remediation"`
}

func TestWriteJSONConformsToApprovedContract(t *testing.T) {
	schemaData, err := os.ReadFile("../../specs/001-environment-doctor/contracts/doctor-report.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		t.Fatal(err)
	}
	wantIDs := schema.Defs["checkResult"].Properties["id"].Enum
	if len(wantIDs) != 9 {
		t.Fatalf("schema ID enum has %d values", len(wantIDs))
	}
	wantAreas := schema.Defs["checkResult"].Properties["area"].Enum
	if !reflect.DeepEqual(wantAreas, []string{"Environment", "Repository"}) {
		t.Fatalf("schema area enum = %v, want exact shipped values", wantAreas)
	}
	checks := make([]doctor.CheckResult, 0, 9)
	for i, id := range wantIDs {
		area := "Environment"
		if i >= 4 {
			area = "Repository"
		}
		result := doctor.CheckResult{ID: id, Name: "Check " + id, Area: area, Status: doctor.Pass}
		if i == 8 {
			result.Status = doctor.Fail
			result.Explanation = "specific failure"
			result.Remediation = "specific fix"
		}
		checks = append(checks, result)
	}
	var out bytes.Buffer
	if err := WriteJSON(&out, doctor.Report{Checks: checks, Overall: doctor.Fail}); err != nil {
		t.Fatal(err)
	}
	var got decodedReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", out.String(), err)
	}
	if got.Overall != "not_ready" || len(got.Checks) != 9 {
		t.Fatalf("unexpected report: %#v", got)
	}
	gotIDs := make([]string, 0, 9)
	for i, check := range got.Checks {
		gotIDs = append(gotIDs, check.ID)
		wantArea := "Environment"
		if i >= 4 {
			wantArea = "Repository"
		}
		if check.Name == "" || check.Area != wantArea {
			t.Errorf("check %d missing name/area: %#v", i, check)
		}
		if i < 8 && (check.Status != "pass" || check.Explanation != "" || check.Remediation != "") {
			t.Errorf("Pass mapping invalid: %#v", check)
		}
		if i == 8 && (check.Status != "fail" || check.Explanation == "" || check.Remediation == "") {
			t.Errorf("Fail mapping invalid: %#v", check)
		}
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("IDs = %v, want %v", gotIDs, wantIDs)
	}
}

func TestWriteJSONReadyMapping(t *testing.T) {
	var out bytes.Buffer
	if err := WriteJSON(&out, doctor.Report{Overall: doctor.Pass}); err != nil {
		t.Fatal(err)
	}
	var got decodedReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.Overall != "ready" {
		t.Fatalf("output = %q, decoded %#v, err %v", out.String(), got, err)
	}
}
