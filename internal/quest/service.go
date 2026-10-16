package quest

import (
	"context"
	"errors"
	"sort"
	"time"

	"api-library/internal/grant"
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
	// Reset drops all of the user's quest progress (admin only; the route requires the admin role).
	Reset(ctx context.Context, uid string) error

	// Admin panel (see admin.go): any user's progress, by uid.
	AdminCatalog() *Catalog
	AdminGet(ctx context.Context, uid string) (*AdminUserQuests, error)
	AdminSetProgress(ctx context.Context, uid string, req SetProgressRequest) (*AdminProgress, error)
	AdminResetQuest(ctx context.Context, uid, questID string) (bool, error)
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

func (s *service) Reset(ctx context.Context, uid string) error {
	return s.repo.Delete(ctx, uid)
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
	var err error
	// A step that pays gems also writes a grant, in the same transaction; any other step touches the quest
	// document only (no extra read).
	if req.StepIndex >= 0 && req.StepIndex < len(def.Steps) && def.Steps[req.StepIndex].NeedsGrants() {
		err = s.repo.UpdateWithGrants(ctx, uid, func(doc *QuestsDoc, grants *grant.Doc) (bool, bool, error) {
			var write, wroteGrant bool
			var err error
			res, write, wroteGrant, err = s.apply(doc, grants, def, req)
			if write {
				doc.CatalogVersion = s.catalog.Version
				doc.UpdatedAt = s.now()
			}
			return write, wroteGrant, err
		})
	} else {
		err = s.repo.Update(ctx, uid, func(doc *QuestsDoc) (bool, error) {
			// fn may be retried by the transaction, so build the response from scratch each time.
			var write bool
			var err error
			res, write, _, err = s.apply(doc, nil, def, req)
			if write {
				doc.CatalogVersion = s.catalog.Version
				doc.UpdatedAt = s.now()
			}
			return write, err
		})
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

// GemGrantID is the grant a step's Gem reward is stored as. It does not depend on the quest's progress, so a
// quest that an admin reset or that is replayed never pays its gems a second time.
func GemGrantID(questID, stepID string) string { return "quest:" + questID + ":" + stepID }

// apply runs one step against the stored state. grants is only given for a step that pays gems; wroteGrant
// reports that the grants document changed.
func (s *service) apply(doc *QuestsDoc, grants *grant.Doc, def *Definition, req CompleteStepRequest) (res *CompleteStepResponse, write, wroteGrant bool, err error) {
	now := s.now()

	// Rejections never write; a quest created here is only stored if the step is accepted.
	p := doc.Quests[def.ID]
	if p == nil {
		completed := func(id string) bool { q := doc.Quests[id]; return q != nil && q.Completed }
		if !s.catalog.IsUnlockedBy(def.ID, completed) {
			return reject(CodeQuestLocked, nil), false, false, nil
		}
		p = &Progress{QuestID: def.ID, StartedAt: now, UpdatedAt: now}
		doc.Quests[def.ID] = p
	}

	if req.StepIndex < 0 || req.StepIndex >= len(def.Steps) || def.Steps[req.StepIndex].ID != req.StepID {
		return reject(CodeStepMismatch, p), false, false, nil
	}

	if p.HasCompletedStep(req.StepID) {
		return accept(p, nil, nil), false, false, nil
	}

	if p.Completed || req.StepIndex != p.StepIndex {
		return reject(CodeStepMismatch, p), false, false, nil
	}

	step := def.Steps[req.StepIndex]
	p.CompletedSteps = append(p.CompletedSteps, step.ID)
	p.UpdatedAt = now
	granted := make([]GrantedReward, 0, len(step.Rewards))
	var gemRewards, warpRewards, petRewards []grant.Reward
	for _, r := range step.Rewards {
		// Cash, Pull and Unlock are applied by the client from this list. Gems are the server's: they are
		// stored as a grant below, and listed here only when this call paid them. A time warp is a grant the
		// client applies (it needs the client's income), so it is not listed.
		switch r.Type {
		case RewardGem:
			gemRewards = append(gemRewards, grant.Reward{Type: grant.RewardGem, Key: r.Key, Amount: r.Amount})
			continue
		case RewardTimeWarp:
			warpRewards = append(warpRewards, grant.Reward{Type: grant.RewardTimeWarp, Key: r.Key, Amount: r.Amount})
			continue
		case RewardPet:
			// A pet is a grant too: the ledger is where the server keeps the owned list (package pet).
			petRewards = append(petRewards, grant.Reward{Type: grant.RewardPet, Key: r.Key, Amount: 1})
			continue
		}
		granted = append(granted, GrantedReward(r))
	}
	if len(warpRewards) > 0 {
		if grants == nil {
			return nil, false, false, errors.New("quest: a step with a time warp was applied without the grants document")
		}
		// Unclaimed: the client applies it and reports it claimed. Paid once per account, like the gems.
		added, err := grants.Add(grant.Grant{
			ID:        GemGrantID(def.ID, step.ID) + ":warp",
			Source:    grant.SourceQuest,
			SourceRef: def.ID + "/" + step.ID,
			Rewards:   warpRewards,
			CreatedAt: now,
		})
		if err != nil {
			return nil, false, false, err
		}
		if added {
			grants.UpdatedAt = now
			wroteGrant = true
		}
	}
	for _, pr := range petRewards {
		if grants == nil {
			return nil, false, false, errors.New("quest: a step with a pet was applied without the grants document")
		}
		// Unclaimed: the client adds the pet to its owned list and reports the grant claimed. One grant per pet and
		// step, so a reset and replayed quest never pays it twice.
		added, err := grants.Add(grant.Grant{
			ID:        GemGrantID(def.ID, step.ID) + ":pet:" + pr.Key,
			Source:    grant.SourceQuest,
			SourceRef: def.ID + "/" + step.ID,
			Rewards:   []grant.Reward{pr},
			CreatedAt: now,
		})
		if err != nil {
			return nil, false, false, err
		}
		if added {
			grants.UpdatedAt = now
			wroteGrant = true
		}
	}
	if len(gemRewards) > 0 {
		if grants == nil {
			return nil, false, false, errors.New("quest: a step with gems was applied without the grants document")
		}
		// Already paid (the quest was reset and replayed): the grant is kept, nothing is added again.
		added, err := grants.Add(grant.Grant{
			ID:        GemGrantID(def.ID, step.ID),
			Source:    grant.SourceQuest,
			SourceRef: def.ID + "/" + step.ID,
			Rewards:   gemRewards,
			CreatedAt: now,
			// Gems are credited by the gem service whatever the claim state, and the client has nothing to apply.
			ClaimedAt: &now,
		})
		if err != nil {
			return nil, false, false, err
		}
		if added {
			grants.UpdatedAt = now
			wroteGrant = true
			for _, r := range gemRewards {
				granted = append(granted, GrantedReward{Type: RewardGem, Key: r.Key, Amount: r.Amount})
			}
		}
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

	return accept(p, granted, started), true, wroteGrant, nil
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
