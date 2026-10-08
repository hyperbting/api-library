package quest

import (
	"context"
	"sort"
	"time"
)

// Error codes returned in CompleteStepResponse.ErrorCode (HTTP 200, accepted=false).
// The client resyncs to CompleteStepResponse.Quest on any of them.
const (
	CodeUnknownQuest = "unknown_quest"
	CodeQuestLocked  = "quest_locked"
	CodeStepMismatch = "step_mismatch"
)

type Service interface {
	GetProgress(ctx context.Context, uid string) (*GetProgressResponse, error)
	CompleteStep(ctx context.Context, uid string, req CompleteStepRequest) (*CompleteStepResponse, error)
}

type service struct {
	repo    Repository
	catalog *Catalog
	now     func() time.Time
}

func NewService(repo Repository, catalog *Catalog) Service {
	return &service{repo: repo, catalog: catalog, now: time.Now}
}

func (s *service) GetProgress(ctx context.Context, uid string) (*GetProgressResponse, error) {
	doc, err := s.repo.Get(ctx, uid)
	if err != nil {
		return nil, err
	}

	res := &GetProgressResponse{Quests: []Progress{}}
	if doc == nil {
		return res, nil
	}

	for _, p := range doc.Quests {
		// Quests removed from the catalog are kept in storage but not sent.
		if _, ok := s.catalog.Get(p.QuestID); ok {
			res.Quests = append(res.Quests, *p)
		}
	}
	sort.Slice(res.Quests, func(i, j int) bool { return res.Quests[i].QuestID < res.Quests[j].QuestID })
	return res, nil
}

// CompleteStep accepts only the quest's current step. A step that was already completed
// is a repeat (retry, or a tutorial replayed from its checkpoint): accepted, nothing granted.
//
// TODO: re-check conditions the server can see (store tier, pet level) once that state is server-side.
// Client-only actions (screen opened, conversation ended) are trusted.
func (s *service) CompleteStep(ctx context.Context, uid string, req CompleteStepRequest) (*CompleteStepResponse, error) {
	def, ok := s.catalog.Get(req.QuestID)
	if !ok {
		return reject(CodeUnknownQuest, nil), nil
	}

	var res *CompleteStepResponse
	err := s.repo.Update(ctx, uid, func(doc *QuestsDoc) (bool, error) {
		// fn may be retried by the transaction, so build the response from scratch each time.
		var write bool
		var err error
		res, write, err = s.apply(doc, def, req)
		if write {
			doc.CatalogVersion = s.catalog.Version
			doc.UpdatedAt = s.now()
		}
		return write, err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *service) apply(doc *QuestsDoc, def *Definition, req CompleteStepRequest) (*CompleteStepResponse, bool, error) {
	now := s.now()

	// Rejections never write; a quest created here is only stored if the step is accepted.
	p := doc.Quests[def.ID]
	if p == nil {
		completed := func(id string) bool { q := doc.Quests[id]; return q != nil && q.Completed }
		if !s.catalog.IsUnlockedBy(def.ID, completed) {
			return reject(CodeQuestLocked, nil), false, nil
		}
		p = &Progress{QuestID: def.ID, StartedAt: now, UpdatedAt: now}
		doc.Quests[def.ID] = p
	}

	if req.StepIndex < 0 || req.StepIndex >= len(def.Steps) || def.Steps[req.StepIndex].ID != req.StepID {
		return reject(CodeStepMismatch, p), false, nil
	}

	if p.HasCompletedStep(req.StepID) {
		return accept(p, nil, nil), false, nil
	}

	if p.Completed || req.StepIndex != p.StepIndex {
		return reject(CodeStepMismatch, p), false, nil
	}

	step := def.Steps[req.StepIndex]
	p.CompletedSteps = append(p.CompletedSteps, step.ID)
	p.UpdatedAt = now
	granted := make([]GrantedReward, 0, len(step.Rewards))
	for _, r := range step.Rewards {
		// TODO: credit the wallet (users/{uid}/private/wallet) once the server-side economy exists.
		granted = append(granted, GrantedReward(r))
	}

	var started []Progress
	if p.StepIndex+1 < len(def.Steps) {
		p.StepIndex++
		if def.Steps[p.StepIndex].Checkpoint {
			p.CheckpointIndex = p.StepIndex
		}
	} else {
		p.Completed = true
		p.CompletedAt = &now
		if f := def.FollowUp; f != "" && doc.Quests[f] == nil {
			next := &Progress{QuestID: f, StartedAt: now, UpdatedAt: now}
			doc.Quests[f] = next
			started = append(started, *next)
		}
	}

	return accept(p, granted, started), true, nil
}

func accept(p *Progress, granted []GrantedReward, started []Progress) *CompleteStepResponse {
	if granted == nil {
		granted = []GrantedReward{}
	}
	if started == nil {
		started = []Progress{}
	}
	q := *p
	return &CompleteStepResponse{Accepted: true, Quest: &q, Granted: granted, Started: started}
}

func reject(code string, p *Progress) *CompleteStepResponse {
	res := &CompleteStepResponse{ErrorCode: code, Granted: []GrantedReward{}, Started: []Progress{}}
	if p != nil {
		q := *p
		res.Quest = &q
	}
	return res
}
