package aicapabilities

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
)

// Service coordinates AI capabilities with robust deterministic fallbacks.
type Service struct {
	gw  gateway.Client
	log *slog.Logger
}

// NewService creates a new AI capabilities service.
func NewService(gw gateway.Client, log *slog.Logger) *Service {
	return &Service{
		gw:  gw,
		log: log,
	}
}

// MatchProduct matches an input product string against candidates using AI gateway with deterministic fallback.
func (s *Service) MatchProduct(ctx context.Context, req MatchRequest) MatchResponse {
	if s.gw != nil && s.gw.Enabled() {
		inputBytes, err := json.Marshal(req)
		if err == nil {
			gwReq := gateway.Request{
				Capability: gateway.CapProductMatch,
				Input:      string(inputBytes),
			}
			resp, err := s.gw.Invoke(ctx, gwReq)
			if err == nil && resp != nil && resp.Content != "" {
				var matchResp MatchResponse
				if err := json.Unmarshal([]byte(resp.Content), &matchResp); err == nil {
					matchResp.Source = "ai_gateway"
					return matchResp
				}
			}
			s.log.WarnContext(ctx, "gateway capability execution failed, falling back to deterministic", "capability", gateway.CapProductMatch, "err", err)
		}
	}

	// Deterministic Fallback using Arabic string similarity
	normQuery := arabic.Normalize(req.QueryName)
	var bestCandidate string
	var highestScore float64

	for _, cand := range req.Candidates {
		normCand := arabic.Normalize(cand)
		score := arabic.Similarity(normQuery, normCand)
		if score > highestScore {
			highestScore = score
			bestCandidate = cand
		}
	}

	return MatchResponse{
		MatchedCandidate: bestCandidate,
		ConfidenceScore:  highestScore,
		Source:           "deterministic_fallback",
	}
}

// MatchCandidate satisfies the Ingest matcher interface.
func (s *Service) MatchCandidate(ctx context.Context, query string, candidateNames []string) (string, float64) {
	resp := s.MatchProduct(ctx, MatchRequest{
		QueryName:  query,
		Candidates: candidateNames,
	})
	return resp.MatchedCandidate, resp.ConfidenceScore
}

// ExpandSearch expands pharmaceutical query keywords using AI gateway with deterministic fallback.
func (s *Service) ExpandSearch(ctx context.Context, req QueryExpansionRequest) QueryExpansionResponse {
	if s.gw != nil && s.gw.Enabled() {
		gwReq := gateway.Request{
			Capability: gateway.CapSearchExpand,
			Input:      req.Query,
		}
		resp, err := s.gw.Invoke(ctx, gwReq)
		if err == nil && resp != nil && resp.Content != "" {
			var expResp QueryExpansionResponse
			if err := json.Unmarshal([]byte(resp.Content), &expResp); err == nil {
				expResp.Source = "ai_gateway"
				return expResp
			}
		}
	}

	// Deterministic Fallback: normalize Arabic keywords and create basic variations
	clean := arabic.Normalize(req.Query)
	terms := strings.Fields(clean)
	var expanded []string
	for _, t := range terms {
		expanded = append(expanded, t)
		if strings.HasPrefix(t, "ال") && len(t) > 4 {
			expanded = append(expanded, strings.TrimPrefix(t, "ال"))
		}
	}

	return QueryExpansionResponse{
		OriginalTerms: req.Query,
		ExpandedTerms: expanded,
		Source:        "deterministic_fallback",
	}
}
