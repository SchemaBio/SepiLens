package api

import (
	"encoding/json"
	"net/http"
)

type ErrorCode string

const (
	ErrCodeAnnotationUnavailable      ErrorCode = "ANNOTATION_UNAVAILABLE"
	ErrCodeAnnotationStale            ErrorCode = "ANNOTATION_STALE"
	ErrCodePhenotypeUnverified        ErrorCode = "PHENOTYPE_UNVERIFIED"
	ErrCodePhenotypeInconsistent      ErrorCode = "PHENOTYPE_INCONSISTENT"
	ErrCodeGeneDiseaseRefuted         ErrorCode = "GENE_DISEASE_REFUTED"
	ErrCodeAIToolBudgetExceeded       ErrorCode = "AI_TOOL_BUDGET_EXCEEDED"
	ErrCodeAIOutputInvalid            ErrorCode = "AI_OUTPUT_INVALID"
	ErrCodeAuthFailed                 ErrorCode = "AUTH_FAILED"
	ErrCodeUnknownEngine              ErrorCode = "UNKNOWN_ENGINE"
	ErrCodeRulesetNotAvailable        ErrorCode = "RULESET_NOT_AVAILABLE"
	ErrCodeKnowledgeNotLoaded         ErrorCode = "KNOWLEDGE_NOT_LOADED"
	ErrCodeInvalidVariant             ErrorCode = "INVALID_VARIANT"
	ErrCodeInvalidCoordinateSystem    ErrorCode = "INVALID_COORDINATE_SYSTEM"
	ErrCodeAssemblyMismatch           ErrorCode = "ASSEMBLY_MISMATCH"
	ErrCodeUnsupportedVariantOrigin   ErrorCode = "UNSUPPORTED_VARIANT_ORIGIN"
	ErrCodeNeedsClinicalContext       ErrorCode = "NEEDS_CLINICAL_CONTEXT"
	ErrCodeIdempotencyConflict        ErrorCode = "IDEMPOTENCY_CONFLICT"
	ErrCodeJobNotFound                ErrorCode = "JOB_NOT_FOUND"
	ErrCodeJobCancelled               ErrorCode = "CANCELLED"
	ErrCodeProviderCapabilityMismatch ErrorCode = "PROVIDER_CAPABILITY_MISMATCH"
	ErrCodeProviderUnavailable        ErrorCode = "PROVIDER_UNAVAILABLE"
	ErrCodeInternalError              ErrorCode = "INTERNAL_ERROR"
	ErrCodeBadRequest                 ErrorCode = "BAD_REQUEST"
)

type APIError struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	RequestID string    `json:"request_id"`
	Retryable bool      `json:"retryable"`
}

type ErrorEnvelope struct {
	Error APIError `json:"error"`
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, code ErrorCode, msg string, retryable bool) {
	reqID := GetRequestID(r.Context())
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Error: APIError{
			Code:      code,
			Message:   msg,
			RequestID: reqID,
			Retryable: retryable,
		},
	})
}
