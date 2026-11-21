package projectconfig

import (
	"encoding/json"

	"github.com/germanmunozs/ai-dev/internal/agent"
)

// wireRoles fixes the role order: encoding/json sorts map keys.
type wireRoles struct {
	Specification  agent.ID `json:"specification"`
	Planning       agent.ID `json:"planning"`
	Implementation agent.ID `json:"implementation"`
	Review         agent.ID `json:"review"`
	Remediation    agent.ID `json:"remediation"`
}

type wireConfig struct {
	SchemaVersion string     `json:"schema_version"`
	Agents        []agent.ID `json:"agents"`
	Roles         wireRoles  `json:"roles"`
}

// Encode returns the canonical bytes of contracts/config-file.md, identical
// for the same configuration (F011-FR-007, SC-006).
func (c Config) Encode() []byte {
	data, err := json.MarshalIndent(wireConfig{
		SchemaVersion: SchemaVersion,
		Agents:        c.Agents(),
		Roles: wireRoles{
			Specification: c.roles[0], Planning: c.roles[1], Implementation: c.roles[2],
			Review: c.roles[3], Remediation: c.roles[4],
		},
	}, "", "  ")
	if err != nil {
		panic(err) // a fixed struct of strings always encodes
	}
	return append(data, '\n')
}
