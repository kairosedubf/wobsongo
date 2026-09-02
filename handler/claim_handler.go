package handler

import (
	"net/http"

	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/labstack/echo/v4"
)

// ClaimHandler handles HTTP requests related to claim-checking.
type ClaimHandler struct {
	service *service.ClaimService
}

// NewClaimHandler creates a new handler.
func NewClaimHandler(service *service.ClaimService) *ClaimHandler {
	return &ClaimHandler{
		service: service,
	}
}

// @Summary		Check a claim
// @Description	Checks a claim (e.g. from a video transcript) against the knowledge base and returns a cited verdict. Out-of-scope input (not health-related) is a normal 200 response with in_scope=false, not an error.
// @Tags		claims
// @Accept		json
// @Produce		json
// @Param		form	body		dto.CheckClaimDTO	true	"Check Claim Form"
// @Success		200		{object}	model.APIResponse{data=dto.ClaimCheckResponse}
// @Failure		400		{object}	model.APIResponse{error=string}
// @Failure		422		{object}	model.APIResponse{error=string}
// @Failure		500		{object}	model.APIResponse{error=string}
// @Router		/api/v1/claims/check [post]
func (h *ClaimHandler) checkClaimHandler(c echo.Context) error {
	req := new(dto.CheckClaimDTO)
	if err := c.Bind(req); err != nil {
		return &model.APIError{
			Code:     http.StatusBadRequest,
			Internal: err,
			Public:   msgInvalidRequestBody,
		}
	}
	if err := c.Validate(req); err != nil {
		return &model.APIError{
			Code:     http.StatusUnprocessableEntity,
			Internal: err,
			Public:   msgValidationFailed,
		}
	}

	result, err := h.service.CheckClaim(c.Request().Context(), req)
	if err != nil {
		status, message := mapDataErrorToHTTP(err)
		return &model.APIError{Code: status, Internal: err, Public: message}
	}

	return writeJSON(c, http.StatusOK, toClaimCheckResponse(result))
}

// toClaimCheckResponse converts the service-layer result into the wire DTO.
func toClaimCheckResponse(result *service.ClaimCheckResult) dto.ClaimCheckResponse {
	resp := dto.ClaimCheckResponse{
		InScope:          result.InScope,
		RefusalReason:    result.RefusalReason,
		OverallSummary:   result.OverallSummary,
		FormattedMessage: result.FormattedMessage,
	}
	if len(result.SubClaims) == 0 {
		return resp
	}

	resp.SubClaims = make([]dto.SubClaimResponse, len(result.SubClaims))
	for i, sc := range result.SubClaims {
		citations := make([]dto.CitationResponse, len(sc.Citations))
		for j, cit := range sc.Citations {
			citations[j] = dto.CitationResponse{
				Index:      cit.Index,
				Source:     cit.Source,
				DocumentID: cit.DocumentID,
				ChunkID:    cit.ChunkID,
				Text:       cit.Text,
				TableMarkdown: cit.TableMarkdown,
			}
		}
		resp.SubClaims[i] = dto.SubClaimResponse{
			Claim:                   sc.Claim,
			Verdict:                 sc.Verdict.String(),
			Severity:                sc.Severity.String(),
			RecommendMedicalConsult: sc.RecommendMedicalConsult,
			Reasoning:               sc.Reasoning,
			BriefReasoning:          sc.BriefReasoning,
			Citations:               citations,
		}
	}
	return resp
}
