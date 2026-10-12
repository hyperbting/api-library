package quest

import (
	"context"
	"sort"
	"time"
)

// Admin views and edits of any user's quest progress (panel: /admin/quests). Edits set the stored state
// directly: no step conditions are checked and no rewards are granted, so a client that was not told
// about a skipped step never applies its rewards (e.g. the Unlocks of the prologue's early steps).

// AdminProgress is one stored quest, with the fields the player API hides.
type AdminProgress struct {
	QuestID         string     `json:"questId"`
	StepIndex       int        `json:"stepIndex"`
	CheckpointIndex int        `json:"checkpointIndex"`
	Completed       bool       `json:"completed"`
	CompletedSteps  []string   `json:"completedSteps"`
	StartedAt       time.Time  `json:"startedAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
	// False for progress whose quest was removed from the catalog (kept in storage, not sent to players).
	InCatalog bool `json:"inCatalog"`
}

// AdminUserQuests is GET /admin/quests/progress?uid=.
type AdminUserQuests struct {
	UID    string `json:"uid"`
	Exists bool   `json:"exists"`
	// Catalog version the progress was last written against (compare with the catalog's version).
	CatalogVersion int             `json:"catalogVersion"`
	UpdatedAt      *time.Time      `json:"updatedAt,omitempty"`
	Quests         []AdminProgress `json:"quests"`
}

// SetProgressRequest is PUT /admin/quests/progress. Completed marks every step done (StepIndex is ignored);
// otherwise the quest is put at StepIndex with the steps before it completed.
type SetProgressRequest struct {
	UID       string `json:"uid" validate:"required,max=128"`
	QuestID   string `json:"questId" validate:"required,max=64"`
	StepIndex int    `json:"stepIndex" validate:"min=0"`
	Completed bool   `json:"completed"`
}

// ResetQuestRequest is POST /admin/quests/progress/reset.
type ResetQuestRequest struct {
	UID     string `json:"uid" validate:"required,max=128"`
	QuestID string `json:"questId" validate:"required,max=64"`
}

// ResetUserRequest is POST /admin/quests/reset.
type ResetUserRequest struct {
	UID string `json:"uid" validate:"required,max=128"`
}

// ValidUID reports whether uid can name a users/{uid} document: Firebase UIDs, and the email players' ids
// (users/e|<email>). Slashes would address another path.
func ValidUID(uid string) bool {
	if uid == "" || len(uid) > 128 || uid == "." || uid == ".." {
		return false
	}
	for _, r := range uid {
		if r == '/' || r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func (s *service) AdminCatalog() *Catalog { return s.catalog }

func (s *service) AdminGet(ctx context.Context, uid string) (*AdminUserQuests, error) {
	doc, err := s.repo.Get(ctx, uid)
	if err != nil {
		return nil, err
	}

	res := &AdminUserQuests{UID: uid, Quests: []AdminProgress{}}
	if doc == nil {
		return res, nil
	}

	res.Exists = true
	res.CatalogVersion = doc.CatalogVersion
	if !doc.UpdatedAt.IsZero() {
		t := doc.UpdatedAt
		res.UpdatedAt = &t
	}
	for _, p := range doc.Quests {
		res.Quests = append(res.Quests, s.adminProgress(p))
	}
	sort.Slice(res.Quests, func(i, j int) bool { return res.Quests[i].QuestID < res.Quests[j].QuestID })
	return res, nil
}

// AdminSetProgress puts one quest at a step (or completes it), creating its progress when the user has none.
// Completing it starts the follow-up quest when that has no progress, as finishing the last step does.
// Un-completing leaves a follow-up that already started alone.
func (s *service) AdminSetProgress(ctx context.Context, uid string, req SetProgressRequest) (*AdminProgress, error) {
	def, ok := s.catalog.Get(req.QuestID)
	if !ok {
		return nil, ErrUnknownQuest
	}
	if !req.Completed && (req.StepIndex < 0 || req.StepIndex >= len(def.Steps)) {
		return nil, ErrInvalidStep
	}

	var out AdminProgress
	err := s.repo.Update(ctx, uid, func(doc *QuestsDoc) (bool, error) {
		now := s.now()
		p := doc.Quests[def.ID]
		if p == nil {
			p = &Progress{QuestID: def.ID, StartedAt: now}
			doc.Quests[def.ID] = p
		}

		// Rebuilt from the steps, so a retry of this function gives the same result.
		target := req.StepIndex
		if req.Completed {
			target = len(def.Steps) - 1
		}
		done := target
		if req.Completed {
			done = len(def.Steps)
		}
		p.CompletedSteps = make([]string, 0, done)
		for _, st := range def.Steps[:done] {
			p.CompletedSteps = append(p.CompletedSteps, st.ID)
		}
		p.StepIndex = target
		p.CheckpointIndex = 0
		for i := target; i >= 0; i-- {
			if def.Steps[i].Checkpoint {
				p.CheckpointIndex = i
				break
			}
		}
		p.Completed = req.Completed
		p.UpdatedAt = now
		switch {
		case !req.Completed:
			p.CompletedAt = nil
		case p.CompletedAt == nil:
			t := now
			p.CompletedAt = &t
		}

		if f := def.FollowUp; req.Completed && f != "" && doc.Quests[f] == nil {
			doc.Quests[f] = &Progress{QuestID: f, StartedAt: now, UpdatedAt: now}
		}

		doc.CatalogVersion = s.catalog.Version
		doc.UpdatedAt = now
		out = s.adminProgress(p)
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminResetQuest drops one quest's progress, including one the catalog no longer has. It reports whether the
// user had any. An initial quest starts again from step 0 the next time the client opens.
func (s *service) AdminResetQuest(ctx context.Context, uid, questID string) (bool, error) {
	var removed bool
	err := s.repo.Update(ctx, uid, func(doc *QuestsDoc) (bool, error) {
		if _, ok := doc.Quests[questID]; !ok {
			removed = false
			return false, nil
		}
		delete(doc.Quests, questID)
		doc.UpdatedAt = s.now()
		removed = true
		return true, nil
	})
	return removed, err
}

func (s *service) adminProgress(p *Progress) AdminProgress {
	_, inCatalog := s.catalog.Get(p.QuestID)
	steps := append([]string{}, p.CompletedSteps...)
	return AdminProgress{
		QuestID:         p.QuestID,
		StepIndex:       p.StepIndex,
		CheckpointIndex: p.CheckpointIndex,
		Completed:       p.Completed,
		CompletedSteps:  steps,
		StartedAt:       p.StartedAt,
		UpdatedAt:       p.UpdatedAt,
		CompletedAt:     p.CompletedAt,
		InCatalog:       inCatalog,
	}
}
