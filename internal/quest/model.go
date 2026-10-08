package quest

import "time"

// Firestore layout (server-only; security rules deny all client access to users/{uid}/private/*):
//
//	users/{uid}                  public profile (client may read)
//	users/{uid}/private/quests   QuestsDoc, one document per user
//
// One document keeps boot to a single read and lets a step completion, its reward
// idempotency and a follow-up start commit in one transaction.
const (
	UsersCollection   = "users"
	PrivateCollection = "private"
	QuestsDocID       = "quests"

	QuestsSchemaVersion = 1
)

type QuestType string
type EventType string
type RewardType string

// Values match the C# enum names in the Unity client (exported by QuestCatalogExporter).
const (
	QuestTypeTutorial QuestType = "Tutorial"
	QuestTypeChapter  QuestType = "Chapter"
	QuestTypeGoal     QuestType = "Goal"

	EventActionDone        EventType = "ActionDone"
	EventCountIncrement    EventType = "CountIncrement"
	EventStateChanged      EventType = "StateChanged"
	EventAmountEarned      EventType = "AmountEarned"
	EventConversationEnded EventType = "ConversationEnded"

	RewardCash   RewardType = "Cash"
	RewardPull   RewardType = "Pull"
	RewardUnlock RewardType = "Unlock"
)

// Progress is one quest's server-owned state. The JSON form is the API shape
// (mirrors DTO.Quest.QuestProgress in Assets/Scripts/DTO/Quest/QuestDTO.cs).
type Progress struct {
	QuestID         string `firestore:"questId" json:"questId"`
	StepIndex       int    `firestore:"stepIndex" json:"stepIndex"`
	CheckpointIndex int    `firestore:"checkpointIndex" json:"checkpointIndex"`
	Completed       bool   `firestore:"completed" json:"completed"`

	// Step IDs already completed (and rewarded); makes complete-step idempotent.
	CompletedSteps []string  `firestore:"completedSteps" json:"-"`
	StartedAt      time.Time `firestore:"startedAt" json:"-"`
	// Last change; sent so the client can show "last played" (e.g. on an account conflict).
	UpdatedAt   time.Time  `firestore:"updatedAt" json:"updatedAt"`
	CompletedAt *time.Time `firestore:"completedAt,omitempty" json:"-"`
}

func (p *Progress) HasCompletedStep(stepID string) bool {
	for _, id := range p.CompletedSteps {
		if id == stepID {
			return true
		}
	}
	return false
}

// QuestsDoc is stored at users/{uid}/private/quests.
type QuestsDoc struct {
	SchemaVersion int `firestore:"schemaVersion"`
	// Catalog version the progress was last written against.
	CatalogVersion int                  `firestore:"catalogVersion"`
	Quests         map[string]*Progress `firestore:"quests"`
	UpdatedAt      time.Time            `firestore:"updatedAt"`
}

func NewQuestsDoc(catalogVersion int) *QuestsDoc {
	return &QuestsDoc{
		SchemaVersion:  QuestsSchemaVersion,
		CatalogVersion: catalogVersion,
		Quests:         map[string]*Progress{},
	}
}

// API DTOs, mirrored in Assets/Scripts/DTO/Quest/QuestDTO.cs.

// GET /api/{version}/quests/progress
type GetProgressResponse struct {
	Quests []Progress `json:"quests"`
}

// POST /api/{version}/quests/complete-step
type CompleteStepRequest struct {
	QuestID   string `json:"questId" validate:"required,max=64"`
	StepID    string `json:"stepId" validate:"required,max=64"`
	StepIndex int    `json:"stepIndex" validate:"min=0"`
}

type GrantedReward struct {
	Type   RewardType `json:"type"`
	Key    string     `json:"key"`
	Amount float64    `json:"amount"`
}

type CompleteStepResponse struct {
	Accepted  bool            `json:"accepted"`
	Quest     *Progress       `json:"quest"`
	Granted   []GrantedReward `json:"granted"`
	Started   []Progress      `json:"started"`
	ErrorCode string          `json:"errorCode,omitempty"`
}
