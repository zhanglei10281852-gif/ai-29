package requestmeta

import (
	"context"
	"testing"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
)

type principalInterleaveContext struct {
	context.Context
	ready   chan<- struct{}
	release <-chan struct{}
}

func (c principalInterleaveContext) Value(key any) any {
	c.ready <- struct{}{}
	<-c.release
	return c.Context.Value(key)
}

type principalResult struct {
	want domain.Principal
	got  domain.Principal
	ok   bool
}

func TestConcurrentRequestPrincipalsRemainIndependent(t *testing.T) {
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan principalResult, 2)
	base := principalInterleaveContext{Context: context.Background(), ready: ready, release: release}
	principals := []domain.Principal{
		{UserID: "ml-request", Role: domain.RoleMLEngineer},
		{UserID: "review-request", Role: domain.RoleRiskReviewer},
	}

	for _, principal := range principals {
		go func(want domain.Principal) {
			ctx := WithPrincipal(base, want)
			got, ok := Principal(ctx)
			results <- principalResult{want: want, got: got, ok: ok}
		}(principal)
	}

	readyCount := 0
	collected := make([]principalResult, 0, len(principals))
	deadline := time.After(2 * time.Second)
	for readyCount+len(collected) < len(principals) {
		select {
		case <-ready:
			readyCount++
		case result := <-results:
			collected = append(collected, result)
		case <-deadline:
			t.Fatal("concurrent principal binding did not reach a stable state")
		}
	}
	close(release)
	for len(collected) < len(principals) {
		select {
		case result := <-results:
			collected = append(collected, result)
		case <-deadline:
			t.Fatal("concurrent principal binding did not complete")
		}
	}

	for _, result := range collected {
		if !result.ok || result.got.UserID != result.want.UserID || result.got.Role != result.want.Role {
			t.Fatalf("request principal = %+v, want %+v, ok=%v", result.got, result.want, result.ok)
		}
	}
}
