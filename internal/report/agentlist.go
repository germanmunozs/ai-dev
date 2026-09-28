package report

import (
	"encoding/json"
	"fmt"
	"io"

	"ai-dev/internal/agent"
)

// WriteAgentListText writes one stable-order row per supported agent.
func WriteAgentListText(w io.Writer, statuses []agent.AgentStatus) error {
	for _, status := range statuses {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", status.Agent.ID, status.Agent.DisplayLabel, status.Availability); err != nil {
			return err
		}
	}
	return nil
}

// WriteAgentListJSON writes exactly one machine-readable listing object.
func WriteAgentListJSON(w io.Writer, statuses []agent.AgentStatus) error {
	type wireAgent struct {
		ID           string `json:"id"`
		DisplayLabel string `json:"display_label"`
		Availability string `json:"availability"`
	}
	type wireReport struct {
		Agents []wireAgent `json:"agents"`
	}

	agents := make([]wireAgent, 0, len(statuses))
	for _, status := range statuses {
		agents = append(agents, wireAgent{
			ID:           string(status.Agent.ID),
			DisplayLabel: status.Agent.DisplayLabel,
			Availability: status.Availability.String(),
		})
	}
	return json.NewEncoder(w).Encode(wireReport{Agents: agents})
}
