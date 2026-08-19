package audit

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/zhanglei10281852-gif/ai/internal/clock"
	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/identity"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
	"github.com/zhanglei10281852-gif/ai/internal/requestmeta"
)

type Recorder struct {
	clock clock.Clock
}

func NewRecorder(c clock.Clock) Recorder { return Recorder{clock: c} }

func (r Recorder) Record(ctx context.Context, tx repository.Tx, action, entityType, entityID, outcome string, metadata map[string]string) error {
	if err := ValidateAction(action); err != nil {
		return err
	}
	principal, ok := requestmeta.Principal(ctx)
	actor := "system"
	if ok {
		actor = principal.UserID
	}
	metadata = SanitizeMetadata(metadata)
	event := domain.AuditEvent{
		ID:         identity.New("audit"),
		RequestID:  requestmeta.RequestID(ctx),
		Actor:      actor,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Outcome:    NormalizeOutcome(outcome),
		Metadata:   cloneMetadata(metadata),
		CreatedAt:  r.clock.Now(),
	}
	return tx.InsertAuditEvent(ctx, event)
}

func SanitizeMetadata(metadata map[string]string) map[string]string {
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		if key == "" || len(key) > 64 {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) > 512 {
			value = value[:512]
		}
		if strings.ContainsAny(key, "\r\n") || strings.ContainsAny(value, "\r\n") {
			continue
		}
		result[key] = value
	}
	return result
}

func VersionMetadata(before, after int64) map[string]string {
	return map[string]string{
		"version_before": strconv.FormatInt(before, 10),
		"version_after":  strconv.FormatInt(after, 10),
	}
}

func ValidateAction(action string) error {
	trimmed := strings.TrimSpace(action)
	if trimmed == "" || len(trimmed) > 96 || strings.ContainsAny(trimmed, "\r\n") {
		return fmt.Errorf("invalid audit action")
	}
	return nil
}

func NormalizeOutcome(outcome string) string {
	return strings.ToLower(strings.TrimSpace(outcome))
}

func cloneMetadata(metadata map[string]string) map[string]string {
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = value
	}
	return result
}
