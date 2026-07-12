package protocols

import (
	"encoding/json"
	"net/http"
)

type WorkflowResponse struct {
	ProtocolID int64                   `json:"protocolId"`
	Number     string                  `json:"number"`
	Employer   string                  `json:"employer"`
	Stages     []WorkflowStageResponse `json:"stages"`
}

type WorkflowStageResponse struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

func WriteWorkflowJSON(w http.ResponseWriter, response WorkflowResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(response)
}
