package api

import (
	"time"

	"github.com/cpkeller25/cairn/internal/catalog"
	"github.com/cpkeller25/cairn/internal/scorecard"
)

// createServiceRequest is the POST /api/v1/services body.
type createServiceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerTeam   string `json:"owner_team"`
	RepoURL     string `json:"repo_url"`
	Tier        int    `json:"tier"`
}

// serviceResponse is a catalog entry as returned to clients.
type serviceResponse struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	OwnerTeam   string             `json:"owner_team"`
	RepoURL     string             `json:"repo_url"`
	Tier        int                `json:"tier"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	Scorecard   *scorecardResponse `json:"scorecard"`
}

type scorecardResponse struct {
	OverallScore int                   `json:"overall_score"`
	Level        string                `json:"level"`
	Checks       []checkResultResponse `json:"checks"`
}

type checkResultResponse struct {
	Key    string `json:"key"`
	Passed bool   `json:"passed"`
	Weight int    `json:"weight"`
	Detail string `json:"detail"`
}

// checkDefinitionResponse describes a rule, for GET /api/v1/checks.
type checkDefinitionResponse struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Weight      int    `json:"weight"`
}

// listServicesResponse wraps the collection so the payload can grow (paging,
// totals) without becoming a breaking change.
type listServicesResponse struct {
	Services []serviceResponse `json:"services"`
}

type listChecksResponse struct {
	Checks []checkDefinitionResponse `json:"checks"`
}

// errorResponse is the unform error body.
type errorResponse struct {
	Error  string               `json:"error"`
	Fields []catalog.FieldError `json:"fields,omitempty"`
}

// toServiceResponse converts a domain service, plus its optional scorecard,
// into th wire format.
func toServiceResponse(svc catalog.Service, report scorecard.Report, hasReport bool) serviceResponse {
	out := serviceResponse{
		ID:          svc.ID.String(),
		Name:        svc.Name,
		Description: svc.Description,
		OwnerTeam:   svc.OwnerTeam,
		RepoURL:     svc.RepoURL,
		Tier:        svc.Tier,
		CreatedAt:   svc.CreatedAt,
		UpdatedAt:   svc.UpdatedAt,
	}
	if hasReport {
		out.Scorecard = toScorecardResponse(report)
	}
	return out
}

func toScorecardResponse(r scorecard.Report) *scorecardResponse {
	checks := make([]checkResultResponse, 0, len(r.Results))
	for _, res := range r.Results {
		checks = append(checks, checkResultResponse{
			Key:    res.Key,
			Passed: res.Passed,
			Weight: res.Weight,
			Detail: res.Detail,
		})
	}
	return &scorecardResponse{
		OverallScore: r.OverallScore,
		Level:        string(r.Level),
		Checks:       checks,
	}
}
