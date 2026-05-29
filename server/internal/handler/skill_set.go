package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type SkillSetSummaryResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SkillCount  int32  `json:"skill_count"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type SkillSetResponse struct {
	SkillSetSummaryResponse
	Skills []SkillSummaryResponse `json:"skills"`
}

type CreateSkillSetRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	SkillIDs    []string `json:"skill_ids"`
}

func skillSetSummaryToResponse(
	id, workspaceID pgtype.UUID,
	name, description string,
	skillCount int32,
	createdBy pgtype.UUID,
	createdAt, updatedAt pgtype.Timestamptz,
) SkillSetSummaryResponse {
	return SkillSetSummaryResponse{
		ID:          uuidToString(id),
		WorkspaceID: uuidToString(workspaceID),
		Name:        name,
		Description: description,
		SkillCount:  skillCount,
		CreatedBy:   uuidToString(createdBy),
		CreatedAt:   timestampToString(createdAt),
		UpdatedAt:   timestampToString(updatedAt),
	}
}

func skillSetToSummaryResponse(s db.SkillSet, skillCount int32) SkillSetSummaryResponse {
	return skillSetSummaryToResponse(
		s.ID,
		s.WorkspaceID,
		s.Name,
		s.Description,
		skillCount,
		s.CreatedBy,
		s.CreatedAt,
		s.UpdatedAt,
	)
}

func (h *Handler) loadSkillSetForUser(w http.ResponseWriter, r *http.Request, id string) (db.SkillSet, bool) {
	workspaceID := h.resolveWorkspaceID(r)
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id is required")
		return db.SkillSet{}, false
	}

	skillSetUUID, ok := parseUUIDOrBadRequest(w, id, "skill set id")
	if !ok {
		return db.SkillSet{}, false
	}

	skillSet, err := h.Queries.GetSkillSetInWorkspace(r.Context(), db.GetSkillSetInWorkspaceParams{
		ID:          skillSetUUID,
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "skill set not found")
		return skillSet, false
	}
	return skillSet, true
}

func (h *Handler) ListSkillSets(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}

	sets, err := h.Queries.ListSkillSetsByWorkspace(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list skill sets")
		return
	}

	resp := make([]SkillSetSummaryResponse, len(sets))
	for i, s := range sets {
		resp[i] = skillSetSummaryToResponse(
			s.ID,
			s.WorkspaceID,
			s.Name,
			s.Description,
			s.SkillCount,
			s.CreatedBy,
			s.CreatedAt,
			s.UpdatedAt,
		)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateSkillSet(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}
	creatorID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}

	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin", "member"); !ok {
		return
	}

	var req CreateSkillSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	skillUUIDs, ok := parseUUIDSliceOrBadRequest(w, req.SkillIDs, "skill id")
	if !ok {
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.Queries.WithTx(tx)

	skillSet, err := qtx.CreateSkillSet(r.Context(), db.CreateSkillSetParams{
		WorkspaceID: workspaceUUID,
		Name:        sanitizeNullBytes(req.Name),
		Description: sanitizeNullBytes(req.Description),
		CreatedBy:   parseUUID(creatorID),
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a skill set with this name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create skill set")
		return
	}

	for _, skillID := range skillUUIDs {
		if _, err := qtx.GetSkillInWorkspace(r.Context(), db.GetSkillInWorkspaceParams{
			ID:          skillID,
			WorkspaceID: workspaceUUID,
		}); err != nil {
			writeError(w, http.StatusBadRequest, "skill not found")
			return
		}
		if err := qtx.AddSkillSetSkill(r.Context(), db.AddSkillSetSkillParams{
			SkillSetID: skillSet.ID,
			SkillID:    skillID,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to add skill to skill set")
			return
		}
	}

	skills, err := qtx.ListSkillSetSkills(r.Context(), skillSet.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list skill set skills")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit")
		return
	}

	resp := skillSetResponse(skillSet, skills)
	actorType, actorID := h.resolveActor(r, creatorID, workspaceID)
	h.publish(protocol.EventSkillSetCreated, workspaceID, actorType, actorID, map[string]any{"skill_set": resp})
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) GetSkillSet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	skillSet, ok := h.loadSkillSetForUser(w, r, id)
	if !ok {
		return
	}

	skills, err := h.Queries.ListSkillSetSkills(r.Context(), skillSet.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list skill set skills")
		return
	}

	writeJSON(w, http.StatusOK, skillSetResponse(skillSet, skills))
}

func skillSetResponse(skillSet db.SkillSet, skills []db.Skill) SkillSetResponse {
	skillResps := make([]SkillSummaryResponse, len(skills))
	for i, s := range skills {
		skillResps[i] = skillSummaryToResponse(
			s.ID,
			s.WorkspaceID,
			s.Name,
			s.Description,
			s.Config,
			s.CreatedBy,
			s.CreatedAt,
			s.UpdatedAt,
		)
	}
	return SkillSetResponse{
		SkillSetSummaryResponse: skillSetToSummaryResponse(skillSet, int32(len(skills))),
		Skills:                  skillResps,
	}
}

// canManageSkillSet checks whether the current user can manage a skill set.
func (h *Handler) canManageSkillSet(w http.ResponseWriter, r *http.Request, skillSet db.SkillSet) bool {
	wsID := uuidToString(skillSet.WorkspaceID)
	member, ok := h.requireWorkspaceRole(w, r, wsID, "skill set not found", "owner", "admin", "member")
	if !ok {
		return false
	}
	isAdmin := roleAllowed(member.Role, "owner", "admin")
	isCreator := uuidToString(skillSet.CreatedBy) == requestUserID(r)
	if !isAdmin && !isCreator {
		writeError(w, http.StatusForbidden, "only the skill set creator can manage this skill set")
		return false
	}
	return true
}
