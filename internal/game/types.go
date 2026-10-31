package game

import "errors"

// Phase enumerates all discrete states of the Avalon tabletop state machine.
type Phase string

const (
	PhaseLobby          Phase = "LOBBY"
	PhaseTeamProposal   Phase = "TEAM_PROPOSAL"
	PhaseProposalSpeech Phase = "PROPOSAL_SPEECH"
	PhaseTeamVote       Phase = "TEAM_VOTE"
	PhaseQuestExecution Phase = "QUEST_EXECUTION"
	PhaseAssassination  Phase = "ASSASSINATION"
	PhaseGameOver       Phase = "GAME_OVER"
)

// Alignment represents the two opposing factions in Avalon.
type Alignment string

const (
	AlignmentGood Alignment = "GOOD"
	AlignmentEvil Alignment = "EVIL"
)

// Role enumerates all Avalon character cards.
type Role string

const (
	RoleMerlin   Role = "MERLIN"
	RolePercival Role = "PERCIVAL"
	RoleServant  Role = "SERVANT"
	RoleAssassin Role = "ASSASSIN"
	RoleMorgana  Role = "MORGANA"
	RoleMordred  Role = "MORDRED"
	RoleOberon   Role = "OBERON"
	RoleMinion   Role = "MINION"
)

// WinReason enumerates the four canonical game endings.
type WinReason string

const (
	WinReasonGood             WinReason = "GOOD_WIN"
	WinReasonEvilFiveRejects  WinReason = "EVIL_WIN_FIVE_REJECTS"
	WinReasonEvilQuests       WinReason = "EVIL_WIN_QUESTS"
	WinReasonEvilAssassinated WinReason = "EVIL_WIN_ASSASSINATION"
)

var (
	ErrWrongPhase          = errors.New("当前阶段不允许执行该操作")
	ErrForbiddenRole       = errors.New("当前身份或座位无权执行该操作")
	ErrGoodMustPlaySuccess = errors.New("蓝方好人阵营严格禁止投任务失败票（只能提交 success: true）")
	ErrInvalidAction       = errors.New("无效的游戏参数或操作")
)

// RoleOptions controls which optional special roles are included in the deck.
// Remaining Good slots are filled with SERVANT; remaining Evil slots with MINION.
type RoleOptions struct {
	Merlin   bool `json:"merlin"`
	Percival bool `json:"percival"`
	Assassin bool `json:"assassin"`
	Morgana  bool `json:"morgana"`
	Mordred  bool `json:"mordred"`
	Oberon   bool `json:"oberon"`
}

// QuestSpec describes one of the 5 quests on the Avalon board for a given player count.
type QuestSpec struct {
	QuestNumber      int  `json:"questNumber"`
	TeamSize         int  `json:"teamSize"`
	FailsRequired    int  `json:"failsRequired"`
	TwoFailsRequired bool `json:"twoFailsRequired"`
}

// RoomKey uniquely isolates an Avalon table per Teamie group conversation.
type RoomKey struct {
	WorkspaceID    string
	ConversationID string
}

// PublicChatPolicy instructs Eino Agents (and informs humans) whether speaking
// in the Teamie group chat is currently permitted, preventing cross-talk & spoilers.
type PublicChatPolicy struct {
	Allowed              bool   `json:"allowed"`
	PhaseMode            string `json:"phaseMode"`
	ActiveSpeakerSeat    int    `json:"activeSpeakerSeat,omitempty"`
	MustCallFinishSpeech bool   `json:"mustCallFinishSpeech,omitempty"`
	Guidance             string `json:"guidance"`
}

// PrivateVision holds the strictly Fog-of-War-cropped secret knowledge for a single viewer.
type PrivateVision struct {
	// MERLIN only: Evil seats excluding MORDRED (includes OBERON; never reveals specific evil role).
	VisibleEvilSeats []int `json:"visibleEvilSeats,omitempty"`
	// PERCIVAL only: [MERLIN, MORGANA] deterministically shuffled by (gameId, viewerSub).
	MerlinCandidateSeats []int `json:"merlinCandidateSeats,omitempty"`
	// ASSASSIN / MORGANA / MORDRED / MINION only: other Evil seats excluding self and OBERON.
	EvilTeammateSeats []int `json:"evilTeammateSeats,omitempty"`
	// Human/Agent readable explanation of this role's private sight & blind spots.
	VisionSummary string `json:"visionSummary"`
}

// SelfView contains the caller's own seat, secret card, private vision, and pending action flags.
type SelfView struct {
	UserID            string         `json:"userId"`
	DisplayName       string         `json:"displayName"`
	Seated            bool           `json:"seated"`
	Seat              int            `json:"seat,omitempty"`
	IsHost            bool           `json:"isHost,omitempty"`
	IsLeader          bool           `json:"isLeader,omitempty"`
	IsCurrentSpeaker  bool           `json:"isCurrentSpeaker,omitempty"`
	OnQuestTeam       bool           `json:"onQuestTeam,omitempty"`
	Role              Role           `json:"role,omitempty"`
	RoleName          string         `json:"roleName,omitempty"`
	Alignment         Alignment      `json:"alignment,omitempty"`
	PrivateVision     *PrivateVision `json:"privateVision,omitempty"`
	HasVotedTeam      bool           `json:"hasVotedTeam,omitempty"`
	MyTeamVote        *bool          `json:"myTeamVote,omitempty"`
	HasSubmittedQuest bool           `json:"hasSubmittedQuest,omitempty"`
	CanActNow         bool           `json:"canActNow"`
	ActionRequired    string         `json:"actionRequired,omitempty"`
	ActionHint        string         `json:"actionHint,omitempty"`
}

// SeatView represents publicly visible information for one seat around the round table.
// Role and Alignment are strictly omitted until Phase == GAME_OVER.
type SeatView struct {
	Seat              int       `json:"seat"`
	UserID            string    `json:"userId"`
	DisplayName       string    `json:"displayName"`
	IsHost            bool      `json:"isHost"`
	IsLeader          bool      `json:"isLeader"`
	IsCurrentSpeaker  bool      `json:"isCurrentSpeaker"`
	OnProposedTeam    bool      `json:"onProposedTeam"`
	HasSpokenThisTurn bool      `json:"hasSpokenThisTurn"`
	HasVotedTeam      bool      `json:"hasVotedTeam"`
	LastPublicVote    *bool     `json:"lastPublicVote,omitempty"`
	RevealedRole      Role      `json:"revealedRole,omitempty"`
	RevealedRoleName  string    `json:"revealedRoleName,omitempty"`
	RevealedAlignment Alignment `json:"revealedAlignment,omitempty"`
}

// SeatVoteEntry records one seat's public vote once TEAM_VOTE reveals all ballots.
type SeatVoteEntry struct {
	Seat        int    `json:"seat"`
	DisplayName string `json:"displayName"`
	Approve     bool   `json:"approve"`
}

// ProposalRecord permanently archives a completed team proposal & public vote.
type ProposalRecord struct {
	QuestNumber   int             `json:"questNumber"`
	AttemptNumber int             `json:"attemptNumber"`
	LeaderSeat    int             `json:"leaderSeat"`
	TeamSeats     []int           `json:"teamSeats"`
	Votes         []SeatVoteEntry `json:"votes"`
	ApproveCount  int             `json:"approveCount"`
	RejectCount   int             `json:"rejectCount"`
	Approved      bool            `json:"approved"`
	SpeechNotes   []SpeechEntry   `json:"speechNotes,omitempty"`
}

// SpeechEntry records an optional brief speech note/pass event during PROPOSAL_SPEECH.
type SpeechEntry struct {
	Seat        int    `json:"seat"`
	DisplayName string `json:"displayName"`
	Summary     string `json:"summary,omitempty"`
}

// QuestRecord permanently archives a completed quest with strictly anonymous card counts.
type QuestRecord struct {
	QuestNumber      int   `json:"questNumber"`
	TeamSize         int   `json:"teamSize"`
	FailsRequired    int   `json:"failsRequired"`
	TwoFailsRequired bool  `json:"twoFailsRequired"`
	LeaderSeat       int   `json:"leaderSeat"`
	TeamSeats        []int `json:"teamSeats"`
	SuccessCount     int   `json:"successCount"`
	FailCount        int   `json:"failCount"`
	Succeeded        bool  `json:"succeeded"`
}

// QuestProgressSlot represents one of the 5 quest medallions on the board track.
type QuestProgressSlot struct {
	QuestNumber      int    `json:"questNumber"`
	TeamSize         int    `json:"teamSize"`
	FailsRequired    int    `json:"failsRequired"`
	TwoFailsRequired bool   `json:"twoFailsRequired"`
	Status           string `json:"status"` // "PENDING" | "ACTIVE" | "SUCCEEDED" | "FAILED"
	SuccessCount     *int   `json:"successCount,omitempty"`
	FailCount        *int   `json:"failCount,omitempty"`
}

// AssassinationRecord captures the Assassin's endgame shot in GAME_OVER.
type AssassinationRecord struct {
	AssassinSeat int  `json:"assassinSeat"`
	TargetSeat   int  `json:"targetSeat"`
	TargetRole   Role `json:"targetRole"`
	HitMerlin    bool `json:"hitMerlin"`
}

// PlayerView is the complete Fog-of-War-cropped snapshot returned to a specific caller (sub).
type PlayerView struct {
	WorkspaceID          string               `json:"workspaceId"`
	ConversationID       string               `json:"conversationId"`
	GameID               string               `json:"gameId,omitempty"`
	EventSeq             int64                `json:"eventSeq"`
	Phase                Phase                `json:"phase"`
	PhaseLabel           string               `json:"phaseLabel"`
	Preset               string               `json:"preset"`
	TargetPlayerCount    int                  `json:"targetPlayerCount"`
	GoodCount            int                  `json:"goodCount"`
	EvilCount            int                  `json:"evilCount"`
	RoleOptions          RoleOptions          `json:"roleOptions"`
	PhaseTimeoutSec      int                  `json:"phaseTimeoutSec"`
	PhaseDeadlineUnixMs  int64                `json:"phaseDeadlineUnixMs,omitempty"`
	QuestNumber          int                  `json:"questNumber,omitempty"`
	ProposalAttempt      int                  `json:"proposalAttempt,omitempty"`
	RejectedCount        int                  `json:"rejectedCount"`
	IsHammerProposal     bool                 `json:"isHammerProposal,omitempty"`
	LeaderSeat           int                  `json:"leaderSeat,omitempty"`
	CurrentTeamSize      int                  `json:"currentTeamSize,omitempty"`
	ProposedTeamSeats    []int                `json:"proposedTeamSeats,omitempty"`
	SpeechOrder          []int                `json:"speechOrder,omitempty"`
	CurrentSpeakerSeat   int                  `json:"currentSpeakerSeat,omitempty"`
	CurrentSpeechNotes   []SpeechEntry        `json:"currentSpeechNotes,omitempty"`
	QuestSubmittedCount  int                  `json:"questSubmittedCount,omitempty"`
	QuestRequiredCards   int                  `json:"questRequiredCards,omitempty"`
	QuestTrack           []QuestProgressSlot  `json:"questTrack"`
	CompletedQuests      []QuestRecord        `json:"completedQuests"`
	VoteHistory          []ProposalRecord     `json:"voteHistory"`
	SuccessQuestCount    int                  `json:"successQuestCount"`
	FailQuestCount       int                  `json:"failQuestCount"`
	Winner               Alignment            `json:"winner,omitempty"`
	WinReason            WinReason            `json:"winReason,omitempty"`
	WinSummary           string               `json:"winSummary,omitempty"`
	Assassination        *AssassinationRecord `json:"assassination,omitempty"`
	Seats                []SeatView           `json:"seats"`
	You                  SelfView             `json:"you"`
	PublicChatPolicy     PublicChatPolicy     `json:"publicChatPolicy"`
	RecentEvents         []string             `json:"recentEvents"`
}

// ActionResponse wraps the result of any mutating tool/REST call with the caller's updated view.
type ActionResponse struct {
	OK               bool             `json:"ok"`
	EventSeq         int64            `json:"eventSeq"`
	Message          string           `json:"message"`
	Phase            Phase            `json:"phase"`
	PublicChatPolicy PublicChatPolicy `json:"publicChatPolicy"`
	View             PlayerView       `json:"view"`
}

// NotificationPayload is the < 2 KB Fog-of-War JSON-RPC notification body pushed over MCP SSE.
type NotificationPayload struct {
	Event            string           `json:"event"`
	EventSeq         int64            `json:"eventSeq"`
	GameID           string           `json:"gameId,omitempty"`
	Phase            Phase            `json:"phase"`
	QuestNumber      int              `json:"questNumber,omitempty"`
	ProposalAttempt  int              `json:"proposalAttempt,omitempty"`
	RejectedCount    int              `json:"rejectedCount"`
	LeaderSeat       int              `json:"leaderSeat,omitempty"`
	ActiveSpeaker    int              `json:"activeSpeakerSeat,omitempty"`
	ProposedTeam     []int            `json:"proposedTeamSeats,omitempty"`
	YourSeat         int              `json:"yourSeat"`
	YourRole         Role             `json:"yourRole,omitempty"`
	PrivateVision    *PrivateVision   `json:"privateVision,omitempty"`
	ActionRequired   string           `json:"actionRequired,omitempty"`
	Summary          string           `json:"summary"`
	ActionHint       string           `json:"actionHint,omitempty"`
	PublicChatPolicy PublicChatPolicy `json:"publicChatPolicy"`
	Winner           Alignment        `json:"winner,omitempty"`
	WinReason        WinReason        `json:"winReason,omitempty"`
}

// TargetedNotification routes a cropped NotificationPayload strictly to TargetUserID's MCP sessions.
type TargetedNotification struct {
	WorkspaceID    string
	ConversationID string
	TargetUserID   string
	Payload        NotificationPayload
}
