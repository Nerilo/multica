package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestSkillSetCRUD_CreateListAndDetail(t *testing.T) {
	ctx := context.Background()

	skillIDs := make([]string, 0, 2)
	for _, name := range []string{"Review prompts", "Debug playbook"} {
		var skillID string
		if err := testPool.QueryRow(ctx, `
			INSERT INTO skill (workspace_id, name, description, content, created_by)
			VALUES ($1, $2, '', '', $3)
			RETURNING id
		`, testWorkspaceID, name, testUserID).Scan(&skillID); err != nil {
			t.Fatalf("failed to seed skill %q: %v", name, err)
		}
		skillIDs = append(skillIDs, skillID)
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM skill WHERE id = $1`, skillID)
		})
	}

	gotCreatedEvent := make(chan map[string]any, 1)
	testHandler.Bus.Subscribe(protocol.EventSkillSetCreated, func(e events.Event) {
		if e.WorkspaceID != testWorkspaceID {
			return
		}
		if payload, ok := e.Payload.(map[string]any); ok {
			select {
			case gotCreatedEvent <- payload:
			default:
			}
		}
	})

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/skill-sets", map[string]any{
		"name":        "Frontend Review",
		"description": "Reusable review context",
		"skill_ids":   skillIDs,
	})
	testHandler.CreateSkillSet(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateSkillSet: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created SkillSetResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("CreateSkillSet: failed to decode response: %v", err)
	}
	if created.Name != "Frontend Review" {
		t.Fatalf("CreateSkillSet: name = %q", created.Name)
	}
	if created.SkillCount != 2 {
		t.Fatalf("CreateSkillSet: skill_count = %d, want 2", created.SkillCount)
	}
	if len(created.Skills) != 2 {
		t.Fatalf("CreateSkillSet: skills len = %d, want 2", len(created.Skills))
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM skill_set WHERE id = $1`, created.ID)
	})

	select {
	case payload := <-gotCreatedEvent:
		if _, ok := payload["skill_set"]; !ok {
			t.Fatalf("skill-set:created payload missing skill_set: %#v", payload)
		}
	default:
		t.Fatal("CreateSkillSet: expected skill-set:created event")
	}

	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/skill-sets", nil)
	testHandler.ListSkillSets(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListSkillSets: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var list []SkillSetSummaryResponse
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("ListSkillSets: failed to decode response: %v", err)
	}
	found := false
	for _, row := range list {
		if row.ID == created.ID {
			found = true
			if row.SkillCount != 2 {
				t.Fatalf("ListSkillSets: skill_count = %d, want 2", row.SkillCount)
			}
		}
	}
	if !found {
		t.Fatalf("ListSkillSets: created skill set %s not found in response", created.ID)
	}

	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/skill-sets/"+created.ID, nil)
	req = withURLParam(req, "id", created.ID)
	testHandler.GetSkillSet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetSkillSet: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var detail SkillSetResponse
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatalf("GetSkillSet: failed to decode response: %v", err)
	}
	if detail.ID != created.ID || len(detail.Skills) != 2 {
		t.Fatalf("GetSkillSet: got id=%s skills=%d, want id=%s skills=2", detail.ID, len(detail.Skills), created.ID)
	}
}
