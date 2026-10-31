package game

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/zzqDeco/teamie-avalon/internal/auth"
)

// Timings configures automatic timeout progression so disconnected players never stall a room.
type Timings struct {
	ProposalTimeout      time.Duration
	SpeakerTurnTimeout   time.Duration
	VoteTimeout          time.Duration
	QuestTimeout         time.Duration
	AssassinationTimeout time.Duration
}

// DefaultTimings returns sensible production timeouts (90s per phase/speaker).
func DefaultTimings() Timings {
	return Timings{
		ProposalTimeout:      90 * time.Second,
		SpeakerTurnTimeout:   90 * time.Second,
		VoteTimeout:          90 * time.Second,
		QuestTimeout:         90 * time.Second,
		AssassinationTimeout: 90 * time.Second,
	}
}

type playerState struct {
	Seat        int
	UserID      string
	DisplayName string
	Role        Role
}

type idemKey struct {
	UserID string
	Key    string
}

type LobbyConfigInput struct {
	Preset            string       `json:"preset,omitempty"`
	TargetPlayerCount int          `json:"targetPlayerCount,omitempty"`
	RoleOptions       *RoleOptions `json:"roleOptions,omitempty"`
	PhaseTimeoutSec   int          `json:"phaseTimeoutSec,omitempty"`
}

type testDeckOverride struct {
	roles         []Role
	initialLeader int
}

// Room manages the thread-safe Avalon state machine for a single (workspaceId, conversationId).
type Room struct {
	mu             sync.Mutex
	workspaceID    string
	conversationID string
	timings        Timings

	eventSeq        int64
	gameID          string
	phase           Phase
	preset          string
	targetCount     int
	roleOptions     RoleOptions
	customRoles     bool
	phaseTimeoutSec int

	players []*playerState // ordered 0..N-1 corresponding to seats 1..N

	questNumber        int
	proposalAttempt    int
	rejectedCount      int
	leaderSeat         int
	proposedTeamSeats  []int
	speechOrder        []int
	speechIdx          int
	currentSpeechNotes []SpeechEntry

	teamVotes       map[int]bool
	lastPublicVotes map[int]bool

	// Strictly anonymous quest ballot box: stores ONLY which seats have submitted
	// and aggregate counts — NEVER stores a seat -> card mapping.
	questSubmittedSeats map[int]struct{}
	questPendingSuccess int
	questPendingFail    int

	completedQuests   []QuestRecord
	voteHistory       []ProposalRecord
	successQuestCount int
	failQuestCount    int

	winner        Alignment
	winReason     WinReason
	winSummary    string
	assassination *AssassinationRecord
	recentEvents  []string

	phaseDeadline time.Time
	phaseTimer    *time.Timer
	timerToken    uint64
	closed        bool

	idemCache    map[idemKey]ActionResponse
	testOverride *testDeckOverride

	onNotifications func([]TargetedNotification)
	onRoomUpdated   func(workspaceID, conversationID string)
}

// NewRoom creates an Avalon room initialized in PhaseLobby.
func NewRoom(
	workspaceID, conversationID string,
	timings Timings,
	onNotifications func([]TargetedNotification),
	onRoomUpdated func(workspaceID, conversationID string),
) *Room {
	_, target, opts, _ := PresetSpec(Preset5P)
	sec := int(timings.SpeakerTurnTimeout / time.Second)
	if sec <= 0 {
		sec = 90
	}
	return &Room{
		workspaceID:         workspaceID,
		conversationID:      conversationID,
		timings:             timings,
		phase:               PhaseLobby,
		preset:              Preset5P,
		targetCount:         target,
		roleOptions:         opts,
		phaseTimeoutSec:     sec,
		teamVotes:           make(map[int]bool),
		lastPublicVotes:     make(map[int]bool),
		questSubmittedSeats: make(map[int]struct{}),
		idemCache:           make(map[idemKey]ActionResponse),
		onNotifications:     onNotifications,
		onRoomUpdated:       onRoomUpdated,
	}
}

// SetTestDeckOverride forces exact role assignment and initial leader on the next StartGame (for unit tests).
func (r *Room) SetTestDeckOverride(roles []Role, initialLeader int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := make([]Role, len(roles))
	copy(copied, roles)
	r.testOverride = &testDeckOverride{
		roles:         copied,
		initialLeader: initialLeader,
	}
}

// Close stops any active phase timer cleanly.
func (r *Room) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.cancelTimerLocked()
}

// GetView returns the Fog-of-War-cropped PlayerView for claims.UserID without mutating state.
func (r *Room) GetView(claims auth.Claims) PlayerView {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buildViewLocked(claims)
}

// JoinLobby seats the caller in PhaseLobby (idempotent if already seated).
func (r *Room) JoinLobby(claims auth.Claims, preset string, idempotencyKey string) (ActionResponse, error) {
	var notifs []TargetedNotification
	var updated bool
	defer func() { r.dispatchSideEffects(notifs, updated) }()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.lookupIdemLocked(claims, idempotencyKey); ok {
		return cached, nil
	}
	if r.phase != PhaseLobby {
		return ActionResponse{}, fmt.Errorf("%w：当前对局已在进行中（%s），请等待本局结束", ErrWrongPhase, r.phase)
	}

	if existing := r.findPlayerByUserIDLocked(claims.UserID); existing != nil {
		if claims.DisplayName != "" && existing.DisplayName != claims.DisplayName {
			existing.DisplayName = claims.DisplayName
			r.bumpSeqLocked()
			updated = true
		}
		if preset != "" && existing.Seat == 1 {
			_ = r.applyPresetLocked(preset)
			r.bumpSeqLocked()
			updated = true
		}
		resp := r.makeActionRespLocked(claims, fmt.Sprintf("你已在 #%d 号座位就座", existing.Seat))
		r.storeIdemLocked(claims, idempotencyKey, resp)
		return resp, nil
	}

	if len(r.players) >= MaxPlayers {
		return ActionResponse{}, fmt.Errorf("%w：圆桌已满 %d 人上限", ErrInvalidAction, MaxPlayers)
	}

	if len(r.players) == 0 && strings.TrimSpace(preset) != "" {
		_ = r.applyPresetLocked(preset)
	}

	seat := len(r.players) + 1
	r.players = append(r.players, &playerState{
		Seat:        seat,
		UserID:      claims.UserID,
		DisplayName: claims.DisplayName,
	})

	// Auto-expand standard preset when more players join than default targetCount (if host didn't lock custom roles).
	if !r.customRoles && len(r.players) > r.targetCount && len(r.players) >= MinPlayers {
		pName, opts := DefaultPresetForCount(len(r.players))
		r.preset = pName
		r.targetCount = len(r.players)
		r.roleOptions = opts
	}

	r.bumpSeqLocked()
	r.appendEventLocked(fmt.Sprintf("%s 入座 #%d 号席位（当前 %d 人）", claims.DisplayName, seat, len(r.players)))
	updated = true

	resp := r.makeActionRespLocked(claims, fmt.Sprintf("成功入座 #%d 号席位", seat))
	r.storeIdemLocked(claims, idempotencyKey, resp)
	return resp, nil
}

// LeaveLobby removes the caller from PhaseLobby and compacts remaining seats 1..K.
func (r *Room) LeaveLobby(claims auth.Claims, idempotencyKey string) (ActionResponse, error) {
	var updated bool
	defer func() { r.dispatchSideEffects(nil, updated) }()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.lookupIdemLocked(claims, idempotencyKey); ok {
		return cached, nil
	}
	if r.phase != PhaseLobby {
		return ActionResponse{}, fmt.Errorf("%w：对局开始后不可中途离座（全员零淘汰）", ErrWrongPhase)
	}

	idx := -1
	for i, p := range r.players {
		if p.UserID == claims.UserID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ActionResponse{}, fmt.Errorf("%w：你当前不在座位上", ErrInvalidAction)
	}

	leaving := r.players[idx]
	r.players = append(r.players[:idx], r.players[idx+1:]...)
	for i, p := range r.players {
		p.Seat = i + 1
	}

	r.bumpSeqLocked()
	r.appendEventLocked(fmt.Sprintf("%s 离开了座位（剩余 %d 人）", leaving.DisplayName, len(r.players)))
	updated = true

	resp := r.makeActionRespLocked(claims, "已离开准备席位")
	r.storeIdemLocked(claims, idempotencyKey, resp)
	return resp, nil
}

// ConfigureLobby lets the room host (Seat 1) adjust preset, special role switches, and timeout in PhaseLobby.
func (r *Room) ConfigureLobby(claims auth.Claims, input LobbyConfigInput, idempotencyKey string) (ActionResponse, error) {
	var updated bool
	defer func() { r.dispatchSideEffects(nil, updated) }()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.lookupIdemLocked(claims, idempotencyKey); ok {
		return cached, nil
	}
	if r.phase != PhaseLobby {
		return ActionResponse{}, fmt.Errorf("%w：仅可在 LOBBY 准备阶段调整板子配置", ErrWrongPhase)
	}

	if len(r.players) > 0 {
		p := r.findPlayerByUserIDLocked(claims.UserID)
		if p == nil || p.Seat != 1 {
			return ActionResponse{}, fmt.Errorf("%w：仅 #1 号房主可修改板子与超时配置", ErrForbiddenRole)
		}
	}

	if err := r.applyConfigInputLocked(input); err != nil {
		return ActionResponse{}, err
	}

	r.bumpSeqLocked()
	r.appendEventLocked(fmt.Sprintf("房主更新了板子配置（预设：%s，推荐 %d 人，超时 %ds）", r.preset, r.targetCount, r.phaseTimeoutSec))
	updated = true

	resp := r.makeActionRespLocked(claims, "已更新房间板子配置")
	r.storeIdemLocked(claims, idempotencyKey, resp)
	return resp, nil
}

// StartGame deals secret roles, computes asymmetric Fog-of-War, and enters Quest 1 TEAM_PROPOSAL.
func (r *Room) StartGame(claims auth.Claims, input LobbyConfigInput, idempotencyKey string) (ActionResponse, error) {
	var notifs []TargetedNotification
	var updated bool
	defer func() { r.dispatchSideEffects(notifs, updated) }()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.lookupIdemLocked(claims, idempotencyKey); ok {
		return cached, nil
	}
	if r.phase != PhaseLobby {
		return ActionResponse{}, fmt.Errorf("%w：当前不在 LOBBY 准备阶段", ErrWrongPhase)
	}

	caller := r.findPlayerByUserIDLocked(claims.UserID)
	if caller == nil {
		return ActionResponse{}, fmt.Errorf("%w：请先调用 join_lobby 入座后再开始游戏", ErrForbiddenRole)
	}

	n := len(r.players)
	if n < MinPlayers || n > MaxPlayers {
		return ActionResponse{}, fmt.Errorf("%w：阿瓦隆需要 %d–%d 名玩家入座才能开局（当前 %d 人）", ErrInvalidAction, MinPlayers, MaxPlayers, n)
	}

	// Host (#1) can apply optional inline config on start_game; non-host can start when table matches targetCount.
	if caller.Seat != 1 && n != r.targetCount {
		return ActionResponse{}, fmt.Errorf("%w：当前入座 %d 人（目标 %d 人），仅 #1 号房主可直接按当前人数开局", ErrForbiddenRole, n, r.targetCount)
	}
	if caller.Seat == 1 && (input.Preset != "" || input.RoleOptions != nil || input.PhaseTimeoutSec > 0 || input.TargetPlayerCount > 0) {
		if err := r.applyConfigInputLocked(input); err != nil {
			return ActionResponse{}, err
		}
	}

	// If host didn't customize roles and seated count differs from preset count, adapt to standard preset for n.
	if !r.customRoles && r.targetCount != n {
		pName, opts := DefaultPresetForCount(n)
		r.preset = pName
		r.targetCount = n
		r.roleOptions = opts
	} else {
		r.targetCount = n
	}

	var deck []Role
	initialLeader := 0
	if r.testOverride != nil && len(r.testOverride.roles) == n {
		deck = make([]Role, n)
		copy(deck, r.testOverride.roles)
		initialLeader = r.testOverride.initialLeader
		r.roleOptions = deriveRoleOptions(deck)
		r.testOverride = nil
	} else {
		built, err := BuildDeck(n, r.roleOptions)
		if err != nil {
			return ActionResponse{}, err
		}
		deck = built
		CryptoShuffleRoles(deck)
	}

	for i, p := range r.players {
		p.Role = deck[i]
	}

	if initialLeader < 1 || initialLeader > n {
		initialLeader = cryptoRandIntn(n) + 1
	}

	r.gameID = uuid.NewString()
	r.phase = PhaseTeamProposal
	r.questNumber = 1
	r.proposalAttempt = 1
	r.rejectedCount = 0
	r.leaderSeat = initialLeader
	r.proposedTeamSeats = nil
	r.speechOrder = nil
	r.speechIdx = 0
	r.currentSpeechNotes = nil
	r.teamVotes = make(map[int]bool)
	r.lastPublicVotes = make(map[int]bool)
	r.questSubmittedSeats = make(map[int]struct{})
	r.questPendingSuccess = 0
	r.questPendingFail = 0
	r.completedQuests = nil
	r.voteHistory = nil
	r.successQuestCount = 0
	r.failQuestCount = 0
	r.winner = ""
	r.winReason = ""
	r.winSummary = ""
	r.assassination = nil
	r.recentEvents = nil

	r.bumpSeqLocked()
	qSpec := r.currentQuestSpecLocked()
	r.appendEventLocked(fmt.Sprintf("新对局开启（%d 人局），暗牌已发放！第 1 轮任务需 %d 人上车，由队长 #%d 提名车队。", n, qSpec.TeamSize, r.leaderSeat))
	r.scheduleTimerLocked(r.effectiveDurationLocked(r.timings.ProposalTimeout))

	notifs = r.notifyLeaderToProposeLocked("game_started_your_turn_to_propose")
	updated = true

	resp := r.makeActionRespLocked(claims, fmt.Sprintf("对局已开始！本轮由 #%d 号队长提名 %d 人车队", r.leaderSeat, qSpec.TeamSize))
	r.storeIdemLocked(claims, idempotencyKey, resp)
	return resp, nil
}

// ProposeTeam submits the current leader's proposed quest team and enters clockwise PROPOSAL_SPEECH.
func (r *Room) ProposeTeam(claims auth.Claims, teamSeats []int, idempotencyKey string) (ActionResponse, error) {
	var notifs []TargetedNotification
	var updated bool
	defer func() { r.dispatchSideEffects(notifs, updated) }()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.lookupIdemLocked(claims, idempotencyKey); ok {
		return cached, nil
	}
	if r.phase != PhaseTeamProposal {
		return ActionResponse{}, fmt.Errorf("%w：当前不在队长提名车队阶段（%s）", ErrWrongPhase, r.phase)
	}

	caller := r.findPlayerByUserIDLocked(claims.UserID)
	if caller == nil || caller.Seat != r.leaderSeat {
		return ActionResponse{}, fmt.Errorf("%w：仅当前队长 #%d 可提名本轮车队", ErrForbiddenRole, r.leaderSeat)
	}

	spec := r.currentQuestSpecLocked()
	cleanSeats, err := validateTeamSeats(teamSeats, len(r.players), spec.TeamSize)
	if err != nil {
		return ActionResponse{}, err
	}

	notifs = r.enterProposalSpeechLocked(cleanSeats, false)
	updated = true

	resp := r.makeActionRespLocked(claims, fmt.Sprintf("已提名车队 %s，进入顺时针轮流发言阶段（当前持麦：#%d）", formatSeatList(cleanSeats), r.currentSpeakerSeatLocked()))
	r.storeIdemLocked(claims, idempotencyKey, resp)
	return resp, nil
}

// FinishSpeech passes the mic from currentSpeakerSeat to the next clockwise speaker, or opens TEAM_VOTE after leader summarizes.
func (r *Room) FinishSpeech(claims auth.Claims, summary string, idempotencyKey string) (ActionResponse, error) {
	var notifs []TargetedNotification
	var updated bool
	defer func() { r.dispatchSideEffects(notifs, updated) }()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.lookupIdemLocked(claims, idempotencyKey); ok {
		return cached, nil
	}
	if r.phase != PhaseProposalSpeech {
		return ActionResponse{}, fmt.Errorf("%w：当前不在轮流发言阶段（%s）", ErrWrongPhase, r.phase)
	}

	activeSeat := r.currentSpeakerSeatLocked()
	caller := r.findPlayerByUserIDLocked(claims.UserID)
	if caller == nil || caller.Seat != activeSeat {
		return ActionResponse{}, fmt.Errorf("%w：当前持麦玩家为 #%d，请勿抢麦", ErrForbiddenRole, activeSeat)
	}

	note := truncateText(strings.TrimSpace(summary), 100)
	notifs = r.advanceSpeechLocked(caller, note, false)
	updated = true

	msg := "已结束发言并交麦给下一位玩家"
	if r.phase == PhaseTeamVote {
		msg = "总结陈词结束，现进入全员组队公投阶段（请调用 vote_team 投赞成或反对）"
	}
	resp := r.makeActionRespLocked(claims, msg)
	r.storeIdemLocked(claims, idempotencyKey, resp)
	return resp, nil
}

// VoteTeam casts a secret approve/reject ballot for the proposed team; reveals all votes once everyone has voted.
func (r *Room) VoteTeam(claims auth.Claims, approve bool, idempotencyKey string) (ActionResponse, error) {
	var notifs []TargetedNotification
	var updated bool
	defer func() { r.dispatchSideEffects(notifs, updated) }()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.lookupIdemLocked(claims, idempotencyKey); ok {
		return cached, nil
	}
	if r.phase != PhaseTeamVote {
		return ActionResponse{}, fmt.Errorf("%w：当前不在车队公投阶段（%s）", ErrWrongPhase, r.phase)
	}

	caller := r.findPlayerByUserIDLocked(claims.UserID)
	if caller == nil {
		return ActionResponse{}, fmt.Errorf("%w：你不在本局座位上，无法参与公投", ErrForbiddenRole)
	}
	if _, already := r.teamVotes[caller.Seat]; already {
		return ActionResponse{}, fmt.Errorf("%w：你已完成本次车队公投，请等待其余玩家落票", ErrInvalidAction)
	}

	r.teamVotes[caller.Seat] = approve
	r.bumpSeqLocked()
	updated = true

	if len(r.teamVotes) == len(r.players) {
		notifs = r.resolveTeamVoteLocked()
	}

	resp := r.makeActionRespLocked(claims, "已提交组队公投票")
	r.storeIdemLocked(claims, idempotencyKey, resp)
	return resp, nil
}

// SubmitQuestCard anonymously submits a quest card (Good players MUST submit success=true).
func (r *Room) SubmitQuestCard(claims auth.Claims, success bool, idempotencyKey string) (ActionResponse, error) {
	var notifs []TargetedNotification
	var updated bool
	defer func() { r.dispatchSideEffects(notifs, updated) }()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.lookupIdemLocked(claims, idempotencyKey); ok {
		return cached, nil
	}
	if r.phase != PhaseQuestExecution {
		return ActionResponse{}, fmt.Errorf("%w：当前不在任务执行阶段（%s）", ErrWrongPhase, r.phase)
	}

	caller := r.findPlayerByUserIDLocked(claims.UserID)
	if caller == nil || !slices.Contains(r.proposedTeamSeats, caller.Seat) {
		return ActionResponse{}, fmt.Errorf("%w：仅本轮入选车队的成员 %s 可提交任务卡", ErrForbiddenRole, formatSeatList(r.proposedTeamSeats))
	}
	if _, already := r.questSubmittedSeats[caller.Seat]; already {
		return ActionResponse{}, fmt.Errorf("%w：你已提交过本轮任务卡，请等待队友出牌", ErrInvalidAction)
	}

	// Hard server-side anti-cheat enforcement: Blue/Good players can NEVER play Fail.
	if RoleAlignment(caller.Role) == AlignmentGood && !success {
		return ActionResponse{}, ErrGoodMustPlaySuccess
	}

	r.questSubmittedSeats[caller.Seat] = struct{}{}
	if success {
		r.questPendingSuccess++
	} else {
		r.questPendingFail++
	}
	r.bumpSeqLocked()
	updated = true

	if len(r.questSubmittedSeats) == len(r.proposedTeamSeats) {
		notifs = r.resolveQuestExecutionLocked()
	}

	resp := r.makeActionRespLocked(claims, "已匿名提交本轮任务卡")
	r.storeIdemLocked(claims, idempotencyKey, resp)
	return resp, nil
}

// Assassinate executes the Assassin's endgame shot to snipe Merlin.
func (r *Room) Assassinate(claims auth.Claims, targetSeat int, idempotencyKey string) (ActionResponse, error) {
	var notifs []TargetedNotification
	var updated bool
	defer func() { r.dispatchSideEffects(notifs, updated) }()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.lookupIdemLocked(claims, idempotencyKey); ok {
		return cached, nil
	}
	if r.phase != PhaseAssassination {
		return ActionResponse{}, fmt.Errorf("%w：当前不在终局刺客拍刀阶段（%s）", ErrWrongPhase, r.phase)
	}

	caller := r.findPlayerByUserIDLocked(claims.UserID)
	if caller == nil || caller.Role != RoleAssassin {
		return ActionResponse{}, fmt.Errorf("%w：仅本局红方刺客（ASSASSIN）可执行拍刀刺杀", ErrForbiddenRole)
	}
	if targetSeat < 1 || targetSeat > len(r.players) {
		return ActionResponse{}, fmt.Errorf("%w：目标座位 #%d 超出有效范围 1..%d", ErrInvalidAction, targetSeat, len(r.players))
	}
	if targetSeat == caller.Seat {
		return ActionResponse{}, fmt.Errorf("%w：刺客不能刺杀自己，请选择疑似梅林的玩家座位", ErrInvalidAction)
	}

	target := r.players[targetSeat-1]
	// Known non-Oberon evil teammates cannot be targeted; Oberon is unknown to Assassin so targeting Oberon is allowed (and misses Merlin).
	if RoleAlignment(target.Role) == AlignmentEvil && target.Role != RoleOberon {
		return ActionResponse{}, fmt.Errorf("%w：#%d 是你的已知红方同伴，请选择疑似蓝方梅林的座位", ErrInvalidAction, targetSeat)
	}

	notifs = r.resolveAssassinationLocked(caller, target, false)
	updated = true

	resp := r.makeActionRespLocked(claims, r.winSummary)
	r.storeIdemLocked(claims, idempotencyKey, resp)
	return resp, nil
}

// ResetLobby resets a finished GAME_OVER room back to LOBBY while keeping seated players ready for a rematch.
func (r *Room) ResetLobby(claims auth.Claims, idempotencyKey string) (ActionResponse, error) {
	var updated bool
	defer func() { r.dispatchSideEffects(nil, updated) }()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.lookupIdemLocked(claims, idempotencyKey); ok {
		return cached, nil
	}
	if r.phase == PhaseLobby {
		resp := r.makeActionRespLocked(claims, "房间已处于 LOBBY 准备阶段")
		r.storeIdemLocked(claims, idempotencyKey, resp)
		return resp, nil
	}
	if r.phase != PhaseGameOver {
		return ActionResponse{}, fmt.Errorf("%w：仅在对局结算（GAME_OVER）后可调用 reset_lobby 重开下一局", ErrWrongPhase)
	}

	r.cancelTimerLocked()
	for _, p := range r.players {
		p.Role = ""
	}
	r.gameID = ""
	r.phase = PhaseLobby
	r.questNumber = 0
	r.proposalAttempt = 0
	r.rejectedCount = 0
	r.leaderSeat = 0
	r.proposedTeamSeats = nil
	r.speechOrder = nil
	r.speechIdx = 0
	r.currentSpeechNotes = nil
	r.teamVotes = make(map[int]bool)
	r.lastPublicVotes = make(map[int]bool)
	r.questSubmittedSeats = make(map[int]struct{})
	r.questPendingSuccess = 0
	r.questPendingFail = 0
	r.completedQuests = nil
	r.voteHistory = nil
	r.successQuestCount = 0
	r.failQuestCount = 0
	r.winner = ""
	r.winReason = ""
	r.winSummary = ""
	r.assassination = nil
	r.idemCache = make(map[idemKey]ActionResponse)

	r.bumpSeqLocked()
	r.appendEventLocked(fmt.Sprintf("%s 重置了圆桌准备台，原座位玩家已保留，可直接开始下一局", claims.DisplayName))
	updated = true

	resp := r.makeActionRespLocked(claims, "已重置为准备大厅，可调整人数/板子或直接开启下一局")
	r.storeIdemLocked(claims, idempotencyKey, resp)
	return resp, nil
}

// --- Internal state transitions (must hold r.mu) ---

func (r *Room) enterProposalSpeechLocked(teamSeats []int, autoTimeout bool) []TargetedNotification {
	r.proposedTeamSeats = teamSeats
	n := len(r.players)
	order := make([]int, 0, n)
	for i := 1; i <= n; i++ {
		seat := ((r.leaderSeat - 1 + i) % n) + 1
		order = append(order, seat)
	}
	r.speechOrder = order
	r.speechIdx = 0
	r.currentSpeechNotes = nil
	r.phase = PhaseProposalSpeech
	r.bumpSeqLocked()

	prefix := ""
	if autoTimeout {
		prefix = "【超时自动提名】"
	}
	r.appendEventLocked(fmt.Sprintf(
		"%s第 %d 轮第 %d 次发车：队长 #%d 提名车队 %s，请从 #%d 开始顺时针发言（队长 #%d 末置位总结）。",
		prefix, r.questNumber, r.proposalAttempt, r.leaderSeat, formatSeatList(teamSeats), order[0], r.leaderSeat,
	))
	r.scheduleTimerLocked(r.effectiveDurationLocked(r.timings.SpeakerTurnTimeout))
	return r.notifyCurrentSpeakerLocked()
}

func (r *Room) advanceSpeechLocked(speaker *playerState, note string, autoTimeout bool) []TargetedNotification {
	entryNote := note
	if autoTimeout && entryNote == "" {
		entryNote = "（超时自动过麦）"
	}
	r.currentSpeechNotes = append(r.currentSpeechNotes, SpeechEntry{
		Seat:        speaker.Seat,
		DisplayName: speaker.DisplayName,
		Summary:     entryNote,
	})

	r.speechIdx++
	r.bumpSeqLocked()

	if r.speechIdx < len(r.speechOrder) {
		nextSeat := r.speechOrder[r.speechIdx]
		if entryNote != "" {
			r.appendEventLocked(fmt.Sprintf("#%d %s 发言完毕（%s），交麦给 #%d", speaker.Seat, speaker.DisplayName, entryNote, nextSeat))
		} else {
			r.appendEventLocked(fmt.Sprintf("#%d %s 发言完毕，交麦给 #%d", speaker.Seat, speaker.DisplayName, nextSeat))
		}
		r.scheduleTimerLocked(r.effectiveDurationLocked(r.timings.SpeakerTurnTimeout))
		return r.notifyCurrentSpeakerLocked()
	}

	// Leader finished summary speech -> enter TEAM_VOTE
	r.phase = PhaseTeamVote
	r.teamVotes = make(map[int]bool)
	r.appendEventLocked(fmt.Sprintf("全员发言结束！请全体玩家对车队 %s 进行赞成/反对公投（流票计数 %d/5）。", formatSeatList(r.proposedTeamSeats), r.rejectedCount))
	r.scheduleTimerLocked(r.effectiveDurationLocked(r.timings.VoteTimeout))
	return r.notifyAllToVoteTeamLocked()
}

func (r *Room) resolveTeamVoteLocked() []TargetedNotification {
	r.cancelTimerLocked()
	n := len(r.players)
	votes := make([]SeatVoteEntry, 0, n)
	approveCount := 0
	rejectCount := 0
	approveSeats := make([]int, 0, n)
	rejectSeats := make([]int, 0, n)

	r.lastPublicVotes = make(map[int]bool, n)
	for _, p := range r.players {
		v := r.teamVotes[p.Seat]
		r.lastPublicVotes[p.Seat] = v
		if v {
			approveCount++
			approveSeats = append(approveSeats, p.Seat)
		} else {
			rejectCount++
			rejectSeats = append(rejectSeats, p.Seat)
		}
		votes = append(votes, SeatVoteEntry{
			Seat:        p.Seat,
			DisplayName: p.DisplayName,
			Approve:     v,
		})
	}

	// Strict majority (> n/2); ties are rejected.
	approved := approveCount > n/2
	notesCopy := make([]SpeechEntry, len(r.currentSpeechNotes))
	copy(notesCopy, r.currentSpeechNotes)
	teamCopy := make([]int, len(r.proposedTeamSeats))
	copy(teamCopy, r.proposedTeamSeats)

	r.voteHistory = append(r.voteHistory, ProposalRecord{
		QuestNumber:   r.questNumber,
		AttemptNumber: r.proposalAttempt,
		LeaderSeat:    r.leaderSeat,
		TeamSeats:     teamCopy,
		Votes:         votes,
		ApproveCount:  approveCount,
		RejectCount:   rejectCount,
		Approved:      approved,
		SpeechNotes:   notesCopy,
	})
	r.teamVotes = make(map[int]bool)

	if approved {
		r.rejectedCount = 0
		r.phase = PhaseQuestExecution
		r.questSubmittedSeats = make(map[int]struct{})
		r.questPendingSuccess = 0
		r.questPendingFail = 0
		r.appendEventLocked(fmt.Sprintf(
			"公投通过（赞成 %d %s / 反对 %d %s）！车队 %s 发车执行第 %d 轮任务。",
			approveCount, formatSeatList(approveSeats), rejectCount, formatSeatList(rejectSeats),
			formatSeatList(teamCopy), r.questNumber,
		))
		r.scheduleTimerLocked(r.effectiveDurationLocked(r.timings.QuestTimeout))
		return r.notifyQuestTeamLocked()
	}

	// Proposal rejected
	r.rejectedCount++
	r.appendEventLocked(fmt.Sprintf(
		"公投否决（赞成 %d %s / 反对 %d %s）！本轮流票计数升至 %d/5。",
		approveCount, formatSeatList(approveSeats), rejectCount, formatSeatList(rejectSeats), r.rejectedCount,
	))

	if r.rejectedCount >= 5 {
		r.phase = PhaseGameOver
		r.winner = AlignmentEvil
		r.winReason = WinReasonEvilFiveRejects
		r.winSummary = fmt.Sprintf("第 %d 轮任务连续 5 次组队被否决（5/5 流票），红方阵营不战而胜！", r.questNumber)
		r.appendEventLocked(r.winSummary)
		return r.notifyGameOverLocked()
	}

	r.proposalAttempt = r.rejectedCount + 1
	r.leaderSeat = (r.leaderSeat % n) + 1
	r.proposedTeamSeats = nil
	r.speechOrder = nil
	r.speechIdx = 0
	r.currentSpeechNotes = nil
	r.phase = PhaseTeamProposal

	hammerNote := ""
	if r.rejectedCount == 4 {
		hammerNote = "【尾置位第 5 车 Hammer 警报：本次若再被否决，红方将直接获胜！】"
	}
	r.appendEventLocked(fmt.Sprintf("队长顺时针移交至 #%d，开始第 %d 轮第 %d 次组队提名。%s", r.leaderSeat, r.questNumber, r.proposalAttempt, hammerNote))
	r.scheduleTimerLocked(r.effectiveDurationLocked(r.timings.ProposalTimeout))
	return r.notifyLeaderToProposeLocked("proposal_rejected_your_turn_to_propose")
}

func (r *Room) resolveQuestExecutionLocked() []TargetedNotification {
	r.cancelTimerLocked()
	spec := r.currentQuestSpecLocked()
	succ := r.questPendingSuccess
	fail := r.questPendingFail
	succeeded := fail < spec.FailsRequired

	teamCopy := make([]int, len(r.proposedTeamSeats))
	copy(teamCopy, r.proposedTeamSeats)

	r.completedQuests = append(r.completedQuests, QuestRecord{
		QuestNumber:      r.questNumber,
		TeamSize:         spec.TeamSize,
		FailsRequired:    spec.FailsRequired,
		TwoFailsRequired: spec.TwoFailsRequired,
		LeaderSeat:       r.leaderSeat,
		TeamSeats:        teamCopy,
		SuccessCount:     succ,
		FailCount:        fail,
		Succeeded:        succeeded,
	})

	// Clear ballot box immediately.
	r.questSubmittedSeats = make(map[int]struct{})
	r.questPendingSuccess = 0
	r.questPendingFail = 0

	if succeeded {
		r.successQuestCount++
		protMsg := ""
		if fail > 0 && spec.TwoFailsRequired {
			protMsg = "（第 4 轮保护轮需至少 2 张失败票才算破坏成功）"
		}
		r.appendEventLocked(fmt.Sprintf(
			"第 %d 轮任务成功！匿名开票：成功 %d 张 / 失败 %d 张%s（总比分 蓝 %d : 红 %d）。",
			r.questNumber, succ, fail, protMsg, r.successQuestCount, r.failQuestCount,
		))
	} else {
		r.failQuestCount++
		r.appendEventLocked(fmt.Sprintf(
			"第 %d 轮任务惨遭破坏！匿名开票：成功 %d 张 / 失败 %d 张（总比分 蓝 %d : 红 %d）。",
			r.questNumber, succ, fail, r.successQuestCount, r.failQuestCount,
		))
	}

	if r.failQuestCount >= 3 {
		r.phase = PhaseGameOver
		r.winner = AlignmentEvil
		r.winReason = WinReasonEvilQuests
		r.winSummary = fmt.Sprintf("红方成功破坏 3 轮任务（蓝 %d : 红 %d），莫德雷德红方阵营获胜！", r.successQuestCount, r.failQuestCount)
		r.appendEventLocked(r.winSummary)
		return r.notifyGameOverLocked()
	}

	if r.successQuestCount >= 3 {
		assassinSeat := r.findRoleSeatLocked(RoleAssassin)
		if assassinSeat > 0 {
			r.phase = PhaseAssassination
			r.appendEventLocked(fmt.Sprintf("蓝方率先完成 3 轮任务！进入终局【刺客拍刀】阶段：请红方简短商议后由刺客执行最后一击（蓝方请保持沉默）。"))
			r.scheduleTimerLocked(r.effectiveDurationLocked(r.timings.AssassinationTimeout))
			return r.notifyAssassinationLocked()
		}

		r.phase = PhaseGameOver
		r.winner = AlignmentGood
		r.winReason = WinReasonGood
		r.winSummary = fmt.Sprintf("蓝方成功完成 3 轮任务（本局无刺客），亚瑟蓝方阵营获胜！")
		r.appendEventLocked(r.winSummary)
		return r.notifyGameOverLocked()
	}

	// Continue to next quest
	n := len(r.players)
	r.questNumber++
	r.proposalAttempt = 1
	r.rejectedCount = 0
	r.leaderSeat = (r.leaderSeat % n) + 1
	r.proposedTeamSeats = nil
	r.speechOrder = nil
	r.speechIdx = 0
	r.currentSpeechNotes = nil
	r.phase = PhaseTeamProposal

	nextSpec := r.currentQuestSpecLocked()
	protHint := ""
	if nextSpec.TwoFailsRequired {
		protHint = "（注意：本轮为保护轮，需至少 2 张失败票才算任务失败）"
	}
	r.appendEventLocked(fmt.Sprintf("进入第 %d 轮任务（需 %d 人上车）%s，队长顺时针轮换至 #%d。", r.questNumber, nextSpec.TeamSize, protHint, r.leaderSeat))
	r.scheduleTimerLocked(r.effectiveDurationLocked(r.timings.ProposalTimeout))
	return r.notifyLeaderToProposeLocked("next_quest_your_turn_to_propose")
}

func (r *Room) resolveAssassinationLocked(assassin *playerState, target *playerState, autoTimeout bool) []TargetedNotification {
	r.cancelTimerLocked()
	hit := target.Role == RoleMerlin
	r.assassination = &AssassinationRecord{
		AssassinSeat: assassin.Seat,
		TargetSeat:   target.Seat,
		TargetRole:   target.Role,
		HitMerlin:    hit,
	}
	r.phase = PhaseGameOver
	r.bumpSeqLocked()

	prefix := ""
	if autoTimeout {
		prefix = "【超时自动拍刀】"
	}
	if hit {
		r.winner = AlignmentEvil
		r.winReason = WinReasonEvilAssassinated
		r.winSummary = fmt.Sprintf("%s刺客 #%d（%s）精准刺中梅林 #%d（%s），红方终局逆转翻盘获胜！", prefix, assassin.Seat, assassin.DisplayName, target.Seat, target.DisplayName)
	} else {
		r.winner = AlignmentGood
		r.winReason = WinReasonGood
		r.winSummary = fmt.Sprintf("%s刺客 #%d（%s）拍刀 #%d（%s，真实身份：%s）未中梅林，蓝方守护梅林最终获胜！", prefix, assassin.Seat, assassin.DisplayName, target.Seat, target.DisplayName, RoleDisplayName(target.Role))
	}
	r.appendEventLocked(r.winSummary)
	return r.notifyGameOverLocked()
}

// --- Timeout handling ---

func (r *Room) scheduleTimerLocked(d time.Duration) {
	r.cancelTimerLocked()
	if r.closed || d <= 0 {
		return
	}
	r.timerToken++
	tok := r.timerToken
	r.phaseDeadline = time.Now().Add(d)
	r.phaseTimer = time.AfterFunc(d, func() {
		r.handleTimeout(tok)
	})
}

func (r *Room) cancelTimerLocked() {
	if r.phaseTimer != nil {
		r.phaseTimer.Stop()
		r.phaseTimer = nil
	}
	r.phaseDeadline = time.Time{}
}

func (r *Room) handleTimeout(tok uint64) {
	var notifs []TargetedNotification
	var updated bool
	defer func() { r.dispatchSideEffects(notifs, updated) }()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.timerToken != tok {
		return
	}

	switch r.phase {
	case PhaseTeamProposal:
		spec := r.currentQuestSpecLocked()
		n := len(r.players)
		autoTeam := make([]int, 0, spec.TeamSize)
		for i := 0; i < spec.TeamSize && i < n; i++ {
			autoTeam = append(autoTeam, ((r.leaderSeat-1+i)%n)+1)
		}
		sort.Ints(autoTeam)
		notifs = r.enterProposalSpeechLocked(autoTeam, true)
		updated = true

	case PhaseProposalSpeech:
		seat := r.currentSpeakerSeatLocked()
		if seat >= 1 && seat <= len(r.players) {
			notifs = r.advanceSpeechLocked(r.players[seat-1], "", true)
			updated = true
		}

	case PhaseTeamVote:
		for _, p := range r.players {
			if _, voted := r.teamVotes[p.Seat]; !voted {
				r.teamVotes[p.Seat] = false
			}
		}
		r.bumpSeqLocked()
		notifs = r.resolveTeamVoteLocked()
		updated = true

	case PhaseQuestExecution:
		for _, seat := range r.proposedTeamSeats {
			if _, done := r.questSubmittedSeats[seat]; !done {
				r.questSubmittedSeats[seat] = struct{}{}
				r.questPendingSuccess++
			}
		}
		r.bumpSeqLocked()
		notifs = r.resolveQuestExecutionLocked()
		updated = true

	case PhaseAssassination:
		aSeat := r.findRoleSeatLocked(RoleAssassin)
		if aSeat >= 1 && aSeat <= len(r.players) {
			assassin := r.players[aSeat-1]
			var target *playerState
			for _, p := range r.players {
				if p.Seat == assassin.Seat {
					continue
				}
				if RoleAlignment(p.Role) == AlignmentEvil && p.Role != RoleOberon {
					continue
				}
				target = p
				break
			}
			if target == nil && len(r.players) > 1 {
				for _, p := range r.players {
					if p.Seat != assassin.Seat {
						target = p
						break
					}
				}
			}
			if target != nil {
				notifs = r.resolveAssassinationLocked(assassin, target, true)
				updated = true
			}
		}
	}
}

// --- Fog-of-War notifications & view construction ---

func (r *Room) notifyLeaderToProposeLocked(eventName string) []TargetedNotification {
	if r.leaderSeat < 1 || r.leaderSeat > len(r.players) {
		return nil
	}
	leader := r.players[r.leaderSeat-1]
	spec := r.currentQuestSpecLocked()
	vision := r.computePlayerVisionLocked(leader)
	policy := r.publicChatPolicyForLocked(leader)

	hammer := ""
	if r.rejectedCount == 4 {
		hammer = "【第 5 次尾置位发车 Hammer：若本次组队被否决，红方将直接获胜！】"
	}
	prot := ""
	if spec.TwoFailsRequired {
		prot = "（本轮为保护轮，需至少 2 张失败票才算失败）"
	}

	return []TargetedNotification{
		{
			WorkspaceID:    r.workspaceID,
			ConversationID: r.conversationID,
			TargetUserID:   leader.UserID,
			Payload: NotificationPayload{
				Event:            eventName,
				EventSeq:         r.eventSeq,
				GameID:           r.gameID,
				Phase:            r.phase,
				QuestNumber:      r.questNumber,
				ProposalAttempt:  r.proposalAttempt,
				RejectedCount:    r.rejectedCount,
				LeaderSeat:       r.leaderSeat,
				YourSeat:         leader.Seat,
				YourRole:         leader.Role,
				PrivateVision:    &vision,
				ActionRequired:   "propose_team",
				Summary:          fmt.Sprintf("轮到你（队长 #%d）提名第 %d 轮任务车队（需选 %d 人）%s。%s", leader.Seat, r.questNumber, spec.TeamSize, prot, hammer),
				ActionHint:       fmt.Sprintf("请立即调用 propose_team 传入 %d 个不重复座位号（1..%d），提名后将进入顺时针发言阶段（你排末位总结）。此时请勿先在公屏抢话。", spec.TeamSize, len(r.players)),
				PublicChatPolicy: policy,
			},
		},
	}
}

func (r *Room) notifyCurrentSpeakerLocked() []TargetedNotification {
	seat := r.currentSpeakerSeatLocked()
	if seat < 1 || seat > len(r.players) {
		return nil
	}
	speaker := r.players[seat-1]
	vision := r.computePlayerVisionLocked(speaker)
	policy := r.publicChatPolicyForLocked(speaker)

	roleCoaching := speakerTacticalHint(speaker.Role, seat == r.leaderSeat)
	return []TargetedNotification{
		{
			WorkspaceID:    r.workspaceID,
			ConversationID: r.conversationID,
			TargetUserID:   speaker.UserID,
			Payload: NotificationPayload{
				Event:            "your_turn_to_speak",
				EventSeq:         r.eventSeq,
				GameID:           r.gameID,
				Phase:            r.phase,
				QuestNumber:      r.questNumber,
				ProposalAttempt:  r.proposalAttempt,
				RejectedCount:    r.rejectedCount,
				LeaderSeat:       r.leaderSeat,
				ActiveSpeaker:    seat,
				ProposedTeam:     append([]int(nil), r.proposedTeamSeats...),
				YourSeat:         speaker.Seat,
				YourRole:         speaker.Role,
				PrivateVision:    &vision,
				ActionRequired:   "finish_speech",
				Summary:          fmt.Sprintf("轮到你（#%d）对队长 #%d 提名的车队 %s 发表看法。", seat, r.leaderSeat, formatSeatList(r.proposedTeamSeats)),
				ActionHint:       roleCoaching + " 请先用 im_send_message 在群聊自然口语简短发言（切忌长篇 Markdown 报告），说完了立即调用 finish_speech 交麦！",
				PublicChatPolicy: policy,
			},
		},
	}
}

func (r *Room) notifyAllToVoteTeamLocked() []TargetedNotification {
	out := make([]TargetedNotification, 0, len(r.players))
	hammer := ""
	if r.rejectedCount == 4 {
		hammer = "（注意：本次为第 5 车 Hammer，若否决则红方直接获胜！）"
	}
	for _, p := range r.players {
		vision := r.computePlayerVisionLocked(p)
		policy := r.publicChatPolicyForLocked(p)
		out = append(out, TargetedNotification{
			WorkspaceID:    r.workspaceID,
			ConversationID: r.conversationID,
			TargetUserID:   p.UserID,
			Payload: NotificationPayload{
				Event:            "your_turn_to_vote_team",
				EventSeq:         r.eventSeq,
				GameID:           r.gameID,
				Phase:            r.phase,
				QuestNumber:      r.questNumber,
				ProposalAttempt:  r.proposalAttempt,
				RejectedCount:    r.rejectedCount,
				LeaderSeat:       r.leaderSeat,
				ProposedTeam:     append([]int(nil), r.proposedTeamSeats...),
				YourSeat:         p.Seat,
				YourRole:         p.Role,
				PrivateVision:    &vision,
				ActionRequired:   "vote_team",
				Summary:          fmt.Sprintf("请对第 %d 轮第 %d 次车队 %s 进行暗票公投%s。", r.questNumber, r.proposalAttempt, formatSeatList(r.proposedTeamSeats), hammer),
				ActionHint:       "请调用 vote_team(approve: true/false) 落票；投票期间严禁在群聊公屏发言拉票。",
				PublicChatPolicy: policy,
			},
		})
	}
	return out
}

func (r *Room) notifyQuestTeamLocked() []TargetedNotification {
	out := make([]TargetedNotification, 0, len(r.proposedTeamSeats))
	spec := r.currentQuestSpecLocked()
	for _, seat := range r.proposedTeamSeats {
		if seat < 1 || seat > len(r.players) {
			continue
		}
		p := r.players[seat-1]
		vision := r.computePlayerVisionLocked(p)
		policy := r.publicChatPolicyForLocked(p)
		hint := "你是蓝方好人，必须调用 submit_quest_card(success: true) 执行任务；严禁公屏发言。"
		if RoleAlignment(p.Role) == AlignmentEvil {
			if spec.TwoFailsRequired {
				hint = "你是红方坏人，本轮为保护轮（需至少 2 张失败票才算破坏成功），请权衡藏身份投 success: true 或炸车投 success: false；严禁公屏发言。"
			} else {
				hint = "你是红方坏人，可自由选择调用 submit_quest_card(success: false) 炸车或传 true 藏身份垫票；严禁公屏发言。"
			}
		}
		out = append(out, TargetedNotification{
			WorkspaceID:    r.workspaceID,
			ConversationID: r.conversationID,
			TargetUserID:   p.UserID,
			Payload: NotificationPayload{
				Event:            "your_turn_to_quest",
				EventSeq:         r.eventSeq,
				GameID:           r.gameID,
				Phase:            r.phase,
				QuestNumber:      r.questNumber,
				ProposalAttempt:  r.proposalAttempt,
				LeaderSeat:       r.leaderSeat,
				ProposedTeam:     append([]int(nil), r.proposedTeamSeats...),
				YourSeat:         p.Seat,
				YourRole:         p.Role,
				PrivateVision:    &vision,
				ActionRequired:   "submit_quest_card",
				Summary:          fmt.Sprintf("你已入选第 %d 轮车队 %s，请立即暗投任务卡。", r.questNumber, formatSeatList(r.proposedTeamSeats)),
				ActionHint:       hint,
				PublicChatPolicy: policy,
			},
		})
	}
	return out
}

func (r *Room) notifyAssassinationLocked() []TargetedNotification {
	out := make([]TargetedNotification, 0, 4)
	for _, p := range r.players {
		if RoleAlignment(p.Role) != AlignmentEvil || p.Role == RoleOberon {
			continue
		}
		vision := r.computePlayerVisionLocked(p)
		policy := r.publicChatPolicyForLocked(p)
		if p.Role == RoleAssassin {
			out = append(out, TargetedNotification{
				WorkspaceID:    r.workspaceID,
				ConversationID: r.conversationID,
				TargetUserID:   p.UserID,
				Payload: NotificationPayload{
					Event:            "your_turn_to_assassinate",
					EventSeq:         r.eventSeq,
					GameID:           r.gameID,
					Phase:            r.phase,
					YourSeat:         p.Seat,
					YourRole:         p.Role,
					PrivateVision:    &vision,
					ActionRequired:   "assassinate",
					Summary:          "蓝方已完成 3 轮任务！现在由你（刺客）执行终局拍刀刺杀梅林。",
					ActionHint:       "可先在群聊简短听取/交换红方同伴意见，然后立即调用 assassinate(targetSeat) 刺杀你认为最像梅林的座位！",
					PublicChatPolicy: policy,
				},
			})
		} else {
			out = append(out, TargetedNotification{
				WorkspaceID:    r.workspaceID,
				ConversationID: r.conversationID,
				TargetUserID:   p.UserID,
				Payload: NotificationPayload{
					Event:            "discuss_assassination",
					EventSeq:         r.eventSeq,
					GameID:           r.gameID,
					Phase:            r.phase,
					YourSeat:         p.Seat,
					YourRole:         p.Role,
					PrivateVision:    &vision,
					Summary:          "进入终局刺客拍刀阶段！你可在群聊用一两句口语帮刺客指出谁的发言和票型最像梅林。",
					ActionHint:       "用 im_send_message 简短给出拍刀建议即可，最终由刺客调用 assassinate 落刀。",
					PublicChatPolicy: policy,
				},
			})
		}
	}
	return out
}

func (r *Room) notifyGameOverLocked() []TargetedNotification {
	out := make([]TargetedNotification, 0, len(r.players))
	for _, p := range r.players {
		policy := r.publicChatPolicyForLocked(p)
		out = append(out, TargetedNotification{
			WorkspaceID:    r.workspaceID,
			ConversationID: r.conversationID,
			TargetUserID:   p.UserID,
			Payload: NotificationPayload{
				Event:            "game_over",
				EventSeq:         r.eventSeq,
				GameID:           r.gameID,
				Phase:            r.phase,
				YourSeat:         p.Seat,
				YourRole:         p.Role,
				Summary:          r.winSummary,
				ActionHint:       "本局已结束，全员身份已公开，可在群聊自由复盘，或调用 reset_lobby 开启下一局。",
				PublicChatPolicy: policy,
				Winner:           r.winner,
				WinReason:        r.winReason,
			},
		})
	}
	return out
}

func speakerTacticalHint(role Role, isLeader bool) string {
	leaderSuffix := ""
	if isLeader {
		leaderSuffix = "你是本轮末置位总结的队长，请回应前序质疑并为自己的车队拉票。"
	}
	switch role {
	case RoleMerlin:
		return "【梅林伪装提醒】你虽知晓红方位置，但绝不能无依据点杀所有坏人，请借历史票型或试探逻辑自然发言，谨防终局被刺客认出！" + leaderSuffix
	case RolePercival:
		return "【派西维尔提醒】请暗中观察两位梅林候选谁在引导好人车队，发言适当替真梅林吸引火力挡刀。" + leaderSuffix
	case RoleMorgana:
		return "【莫甘娜提醒】你在派西维尔眼中是梅林候选之一，请装作稳健带队的梅林视角混淆视听！" + leaderSuffix
	case RoleAssassin:
		return "【刺客提醒】一面伪装好人争取上车，一面留意哪位好人发言对坏人座位异常敏感（可能是梅林）。" + leaderSuffix
	case RoleMordred:
		return "【莫德雷德提醒】梅林看不到你！请大胆以阳光好人姿态争取进车。" + leaderSuffix
	case RoleOberon:
		return "【奥伯伦提醒】你与红方同伴互不可见，请搅浑水或设法混入车队。" + leaderSuffix
	default:
		return "请结合历轮车队构成与公投票型简短盘逻辑表态。" + leaderSuffix
	}
}

func (r *Room) publicChatPolicyForLocked(viewer *playerState) PublicChatPolicy {
	switch r.phase {
	case PhaseLobby:
		return PublicChatPolicy{
			Allowed:   true,
			PhaseMode: "lobby_free_chat",
			Guidance:  "准备阶段可在群聊自由招呼队友入座。",
		}
	case PhaseTeamProposal:
		return PublicChatPolicy{
			Allowed:   false,
			PhaseMode: "silent_team_proposal",
			Guidance:  fmt.Sprintf("正在等待队长 #%d 调用 propose_team 提名车队，请勿在群聊抢话刷屏。", r.leaderSeat),
		}
	case PhaseProposalSpeech:
		activeSeat := r.currentSpeakerSeatLocked()
		if viewer != nil && viewer.Seat == activeSeat {
			return PublicChatPolicy{
				Allowed:              true,
				PhaseMode:            "single_speaker_mic",
				ActiveSpeakerSeat:    activeSeat,
				MustCallFinishSpeech: true,
				Guidance:             fmt.Sprintf("现在轮到你（#%d）持麦！请在群聊用自然短句口语发言（不要发长篇 Markdown 列表），发言后立即调用 finish_speech 交麦。", activeSeat),
			}
		}
		return PublicChatPolicy{
			Allowed:           false,
			PhaseMode:         "single_speaker_mic",
			ActiveSpeakerSeat: activeSeat,
			Guidance:          fmt.Sprintf("当前由 #%d 持麦发言，非持麦玩家严禁在群聊插话。", activeSeat),
		}
	case PhaseTeamVote:
		return PublicChatPolicy{
			Allowed:   false,
			PhaseMode: "silent_team_vote",
			Guidance:  "组队暗票公投进行中，严禁在群聊公屏拉票或透露票向，请直接调用 vote_team。",
		}
	case PhaseQuestExecution:
		return PublicChatPolicy{
			Allowed:   false,
			PhaseMode: "silent_quest_execution",
			Guidance:  "车队正在暗投任务卡，全员严禁公屏发言干扰。",
		}
	case PhaseAssassination:
		if viewer != nil && RoleAlignment(viewer.Role) == AlignmentEvil && viewer.Role != RoleOberon {
			return PublicChatPolicy{
				Allowed:   true,
				PhaseMode: "evil_assassination_discussion",
				Guidance:  "终局拍刀讨论：红方（除奥伯伦外）可在群聊简短交流谁是梅林，最终由刺客调用 assassinate(targetSeat) 落刀。",
			}
		}
		return PublicChatPolicy{
			Allowed:   false,
			PhaseMode: "good_silent_during_assassination",
			Guidance:  "红方刺客正在指认梅林，蓝方与奥伯伦必须保持沉默，严禁场外提示干扰。",
		}
	case PhaseGameOver:
		return PublicChatPolicy{
			Allowed:   true,
			PhaseMode: "post_game_review",
			Guidance:  "本局已结束，全员底牌已揭晓，可在群聊自由复盘。",
		}
	default:
		return PublicChatPolicy{Allowed: false, PhaseMode: "unknown"}
	}
}

func (r *Room) buildViewLocked(claims auth.Claims) PlayerView {
	effCount := r.targetCount
	if len(r.players) >= MinPlayers && (r.phase != PhaseLobby || len(r.players) > effCount) {
		effCount = len(r.players)
	}
	if effCount < MinPlayers {
		effCount = MinPlayers
	}
	if effCount > MaxPlayers {
		effCount = MaxPlayers
	}
	goodCount, evilCount, _ := FactionCounts(effCount)
	specs, _ := QuestMatrix(effCount)

	// Build 5-quest progress track
	completedByNum := make(map[int]QuestRecord, len(r.completedQuests))
	for _, qr := range r.completedQuests {
		completedByNum[qr.QuestNumber] = qr
	}
	questTrack := make([]QuestProgressSlot, 0, len(specs))
	for _, sp := range specs {
		slot := QuestProgressSlot{
			QuestNumber:      sp.QuestNumber,
			TeamSize:         sp.TeamSize,
			FailsRequired:    sp.FailsRequired,
			TwoFailsRequired: sp.TwoFailsRequired,
			Status:           "PENDING",
		}
		if done, ok := completedByNum[sp.QuestNumber]; ok {
			sc := done.SuccessCount
			fc := done.FailCount
			slot.SuccessCount = &sc
			slot.FailCount = &fc
			if done.Succeeded {
				slot.Status = "SUCCEEDED"
			} else {
				slot.Status = "FAILED"
			}
		} else if r.phase != PhaseLobby && r.phase != PhaseGameOver && r.phase != PhaseAssassination && sp.QuestNumber == r.questNumber {
			slot.Status = "ACTIVE"
		}
		questTrack = append(questTrack, slot)
	}

	spokenSet := make(map[int]bool, len(r.currentSpeechNotes))
	for _, sn := range r.currentSpeechNotes {
		spokenSet[sn.Seat] = true
	}

	activeSpeaker := r.currentSpeakerSeatLocked()
	seats := make([]SeatView, 0, len(r.players))
	for _, p := range r.players {
		sv := SeatView{
			Seat:              p.Seat,
			UserID:            p.UserID,
			DisplayName:       p.DisplayName,
			IsHost:            p.Seat == 1,
			IsLeader:          r.phase != PhaseLobby && p.Seat == r.leaderSeat,
			IsCurrentSpeaker:  r.phase == PhaseProposalSpeech && p.Seat == activeSpeaker,
			OnProposedTeam:    slices.Contains(r.proposedTeamSeats, p.Seat),
			HasSpokenThisTurn: spokenSet[p.Seat],
			HasVotedTeam:      r.phase == PhaseTeamVote && hasMapKey(r.teamVotes, p.Seat),
		}
		if lastV, ok := r.lastPublicVotes[p.Seat]; ok && r.phase != PhaseTeamVote {
			vCopy := lastV
			sv.LastPublicVote = &vCopy
		}
		// Anti-cheat hard boundary: NEVER expose Role or Alignment of seats before GAME_OVER.
		if r.phase == PhaseGameOver && p.Role != "" {
			sv.RevealedRole = p.Role
			sv.RevealedRoleName = RoleDisplayName(p.Role)
			sv.RevealedAlignment = RoleAlignment(p.Role)
		}
		seats = append(seats, sv)
	}

	viewer := r.findPlayerByUserIDLocked(claims.UserID)
	policy := r.publicChatPolicyForLocked(viewer)
	you := r.buildSelfViewLocked(claims, viewer, activeSpeaker)

	completedCopy := make([]QuestRecord, len(r.completedQuests))
	copy(completedCopy, r.completedQuests)
	for i := range completedCopy {
		completedCopy[i].TeamSeats = append([]int(nil), completedCopy[i].TeamSeats...)
	}

	votesCopy := make([]ProposalRecord, len(r.voteHistory))
	for i, vh := range r.voteHistory {
		vc := vh
		vc.TeamSeats = append([]int(nil), vh.TeamSeats...)
		vc.Votes = append([]SeatVoteEntry(nil), vh.Votes...)
		vc.SpeechNotes = append([]SpeechEntry(nil), vh.SpeechNotes...)
		votesCopy[i] = vc
	}

	var deadlineMs int64
	if !r.phaseDeadline.IsZero() && r.phase != PhaseLobby && r.phase != PhaseGameOver {
		deadlineMs = r.phaseDeadline.UnixMilli()
	}

	var currentTeamSize int
	var questReqCards int
	if r.questNumber >= 1 && r.questNumber <= len(specs) {
		currentTeamSize = specs[r.questNumber-1].TeamSize
	}
	if r.phase == PhaseQuestExecution {
		questReqCards = len(r.proposedTeamSeats)
	}

	var assassCopy *AssassinationRecord
	if r.assassination != nil {
		c := *r.assassination
		assassCopy = &c
	}

	eventsCopy := make([]string, len(r.recentEvents))
	copy(eventsCopy, r.recentEvents)

	return PlayerView{
		WorkspaceID:         r.workspaceID,
		ConversationID:      r.conversationID,
		GameID:              r.gameID,
		EventSeq:            r.eventSeq,
		Phase:               r.phase,
		PhaseLabel:          phaseChineseLabel(r.phase),
		Preset:              r.preset,
		TargetPlayerCount:   effCount,
		GoodCount:           goodCount,
		EvilCount:           evilCount,
		RoleOptions:         r.roleOptions,
		PhaseTimeoutSec:     r.phaseTimeoutSec,
		PhaseDeadlineUnixMs: deadlineMs,
		QuestNumber:         r.questNumber,
		ProposalAttempt:     r.proposalAttempt,
		RejectedCount:       r.rejectedCount,
		IsHammerProposal:    r.phase != PhaseLobby && r.phase != PhaseGameOver && r.rejectedCount == 4,
		LeaderSeat:          r.leaderSeat,
		CurrentTeamSize:     currentTeamSize,
		ProposedTeamSeats:   append([]int(nil), r.proposedTeamSeats...),
		SpeechOrder:         append([]int(nil), r.speechOrder...),
		CurrentSpeakerSeat:  activeSpeaker,
		CurrentSpeechNotes:  append([]SpeechEntry(nil), r.currentSpeechNotes...),
		QuestSubmittedCount: len(r.questSubmittedSeats),
		QuestRequiredCards:  questReqCards,
		QuestTrack:          questTrack,
		CompletedQuests:     completedCopy,
		VoteHistory:         votesCopy,
		SuccessQuestCount:   r.successQuestCount,
		FailQuestCount:      r.failQuestCount,
		Winner:              r.winner,
		WinReason:           r.winReason,
		WinSummary:          r.winSummary,
		Assassination:       assassCopy,
		Seats:               seats,
		You:                 you,
		PublicChatPolicy:    policy,
		RecentEvents:        eventsCopy,
	}
}

func (r *Room) buildSelfViewLocked(claims auth.Claims, viewer *playerState, activeSpeaker int) SelfView {
	self := SelfView{
		UserID:      claims.UserID,
		DisplayName: claims.DisplayName,
		Seated:      viewer != nil,
	}
	if viewer == nil {
		if r.phase == PhaseLobby && len(r.players) < MaxPlayers {
			self.CanActNow = true
			self.ActionRequired = "join_lobby"
			self.ActionHint = "点击【入座参赛】或调用 join_lobby 加入圆桌。"
		} else if r.phase == PhaseGameOver {
			self.CanActNow = true
			self.ActionRequired = "reset_lobby"
			self.ActionHint = "本局已结束，可重置准备台加入下一局。"
		} else {
			self.ActionHint = "你当前为旁观视角（未入座本局）。"
		}
		return self
	}

	self.Seat = viewer.Seat
	self.IsHost = viewer.Seat == 1
	self.IsLeader = r.phase != PhaseLobby && viewer.Seat == r.leaderSeat
	self.IsCurrentSpeaker = r.phase == PhaseProposalSpeech && viewer.Seat == activeSpeaker
	self.OnQuestTeam = slices.Contains(r.proposedTeamSeats, viewer.Seat)

	if r.phase != PhaseLobby && viewer.Role != "" {
		self.Role = viewer.Role
		self.RoleName = RoleDisplayName(viewer.Role)
		self.Alignment = RoleAlignment(viewer.Role)
		v := r.computePlayerVisionLocked(viewer)
		self.PrivateVision = &v
	}

	switch r.phase {
	case PhaseLobby:
		if len(r.players) >= MinPlayers && (viewer.Seat == 1 || len(r.players) == r.targetCount) {
			self.CanActNow = true
			self.ActionRequired = "start_game"
			self.ActionHint = fmt.Sprintf("已入座 %d 人，可立即发牌开启阿瓦隆对局！", len(r.players))
		} else {
			self.ActionHint = fmt.Sprintf("等待玩家入座（至少 %d 人，当前 %d 人）。", MinPlayers, len(r.players))
		}

	case PhaseTeamProposal:
		spec := r.currentQuestSpecLocked()
		if viewer.Seat == r.leaderSeat {
			self.CanActNow = true
			self.ActionRequired = "propose_team"
			self.ActionHint = fmt.Sprintf("你是本轮队长，请选择 %d 名玩家组成第 %d 轮任务车队。", spec.TeamSize, r.questNumber)
		} else {
			self.ActionHint = fmt.Sprintf("等待队长 #%d 提名 %d 人任务车队。", r.leaderSeat, spec.TeamSize)
		}

	case PhaseProposalSpeech:
		if viewer.Seat == activeSpeaker {
			self.CanActNow = true
			self.ActionRequired = "finish_speech"
			self.ActionHint = "现在轮到你发言！请先在群聊公屏发言，说完后立即点击【结束发言 · 交麦】。"
		} else {
			self.ActionHint = fmt.Sprintf("请听取 #%d 发言（顺时针轮麦，队长 #%d 最后总结）。", activeSpeaker, r.leaderSeat)
		}

	case PhaseTeamVote:
		if myVote, voted := r.teamVotes[viewer.Seat]; voted {
			vCopy := myVote
			self.HasVotedTeam = true
			self.MyTeamVote = &vCopy
			self.ActionHint = fmt.Sprintf("你已投出【%s】，等待其余玩家落票（%d/%d）。", voteText(myVote), len(r.teamVotes), len(r.players))
		} else {
			self.CanActNow = true
			self.ActionRequired = "vote_team"
			self.ActionHint = fmt.Sprintf("请对队长 #%d 提名的车队 %s 投赞成或反对票。", r.leaderSeat, formatSeatList(r.proposedTeamSeats))
		}

	case PhaseQuestExecution:
		if self.OnQuestTeam {
			if _, done := r.questSubmittedSeats[viewer.Seat]; done {
				self.HasSubmittedQuest = true
				self.ActionHint = fmt.Sprintf("你已匿名投入任务卡，等待其余队员出牌（%d/%d）。", len(r.questSubmittedSeats), len(r.proposedTeamSeats))
			} else {
				self.CanActNow = true
				self.ActionRequired = "submit_quest_card"
				if self.Alignment == AlignmentGood {
					self.ActionHint = "你是蓝方好人，请提交【任务成功】卡。"
				} else {
					self.ActionHint = "你是红方坏人，可自由选择【任务破坏（失败）】或【任务成功（伪装）】。"
				}
			}
		} else {
			self.ActionHint = fmt.Sprintf("等待车队成员 %s 匿名执行任务（%d/%d）。", formatSeatList(r.proposedTeamSeats), len(r.questSubmittedSeats), len(r.proposedTeamSeats))
		}

	case PhaseAssassination:
		if viewer.Role == RoleAssassin {
			self.CanActNow = true
			self.ActionRequired = "assassinate"
			self.ActionHint = "你是刺客！请结合整局发言与票型，选择一名疑似梅林的座位执行终局拍刀。"
		} else if self.Alignment == AlignmentEvil && viewer.Role != RoleOberon {
			self.ActionHint = "终局拍刀阶段：你可在群聊简短帮刺客分析谁是梅林，等待刺客落刀。"
		} else {
			self.ActionHint = "等待红方刺客拍刀指认梅林，请保持公屏安静。"
		}

	case PhaseGameOver:
		self.CanActNow = true
		self.ActionRequired = "reset_lobby"
		self.ActionHint = "对局已结算，可点击【再来一局】重置圆桌准备台。"
	}

	return self
}

// --- Helpers ---

func (r *Room) computePlayerVisionLocked(p *playerState) PrivateVision {
	seatRoles := make(map[int]Role, len(r.players))
	for _, pl := range r.players {
		seatRoles[pl.Seat] = pl.Role
	}
	return ComputePrivateVision(r.gameID, p.UserID, p.Seat, seatRoles, r.roleOptions)
}

func (r *Room) applyConfigInputLocked(input LobbyConfigInput) error {
	if input.PhaseTimeoutSec != 0 {
		if input.PhaseTimeoutSec < 5 || input.PhaseTimeoutSec > 300 {
			return fmt.Errorf("%w：阶段超时秒数须在 5–300 秒之间", ErrInvalidAction)
		}
		r.phaseTimeoutSec = input.PhaseTimeoutSec
	}

	if strings.TrimSpace(input.Preset) != "" && strings.ToLower(strings.TrimSpace(input.Preset)) != PresetCustom {
		if err := r.applyPresetLocked(input.Preset); err != nil {
			return err
		}
	}

	if input.TargetPlayerCount != 0 {
		if input.TargetPlayerCount < MinPlayers || input.TargetPlayerCount > MaxPlayers {
			return fmt.Errorf("%w：目标人数须在 %d–%d 之间", ErrInvalidAction, MinPlayers, MaxPlayers)
		}
		r.targetCount = input.TargetPlayerCount
	}

	if input.RoleOptions != nil {
		checkCount := r.targetCount
		if len(r.players) > checkCount {
			checkCount = len(r.players)
		}
		if _, err := BuildDeck(checkCount, *input.RoleOptions); err != nil {
			return err
		}
		r.roleOptions = *input.RoleOptions
		r.customRoles = true
		if strings.TrimSpace(input.Preset) == "" || strings.ToLower(strings.TrimSpace(input.Preset)) == PresetCustom {
			r.preset = PresetCustom
		}
	}
	return nil
}

func (r *Room) applyPresetLocked(raw string) error {
	pName, count, opts, ok := PresetSpec(raw)
	if !ok {
		return fmt.Errorf("%w：不支持的板子预设 %q（可选 5p/6p/7p_oberon/7p_mordred/8p/9p/10p）", ErrInvalidAction, raw)
	}
	r.preset = pName
	r.targetCount = count
	r.roleOptions = opts
	r.customRoles = false
	return nil
}

func (r *Room) effectiveDurationLocked(fallback time.Duration) time.Duration {
	if r.phaseTimeoutSec >= 5 && r.phaseTimeoutSec <= 300 {
		// In unit tests with custom sub-second Timings, preserve fast sub-second test durations.
		if fallback > 0 && fallback < 5*time.Second {
			return fallback
		}
		return time.Duration(r.phaseTimeoutSec) * time.Second
	}
	return fallback
}

func (r *Room) currentQuestSpecLocked() QuestSpec {
	specs, ok := QuestMatrix(len(r.players))
	if !ok || r.questNumber < 1 || r.questNumber > len(specs) {
		return QuestSpec{QuestNumber: 1, TeamSize: 2, FailsRequired: 1}
	}
	return specs[r.questNumber-1]
}

func (r *Room) currentSpeakerSeatLocked() int {
	if r.phase != PhaseProposalSpeech || r.speechIdx < 0 || r.speechIdx >= len(r.speechOrder) {
		return 0
	}
	return r.speechOrder[r.speechIdx]
}

func (r *Room) findPlayerByUserIDLocked(userID string) *playerState {
	for _, p := range r.players {
		if p.UserID == userID {
			return p
		}
	}
	return nil
}

func (r *Room) findRoleSeatLocked(role Role) int {
	for _, p := range r.players {
		if p.Role == role {
			return p.Seat
		}
	}
	return 0
}

func (r *Room) bumpSeqLocked() {
	r.eventSeq++
}

func (r *Room) appendEventLocked(msg string) {
	r.recentEvents = append(r.recentEvents, msg)
	if len(r.recentEvents) > 30 {
		r.recentEvents = r.recentEvents[len(r.recentEvents)-30:]
	}
}

func (r *Room) makeActionRespLocked(claims auth.Claims, message string) ActionResponse {
	view := r.buildViewLocked(claims)
	return ActionResponse{
		OK:               true,
		EventSeq:         r.eventSeq,
		Message:          message,
		Phase:            r.phase,
		PublicChatPolicy: view.PublicChatPolicy,
		View:             view,
	}
}

func (r *Room) lookupIdemLocked(claims auth.Claims, key string) (ActionResponse, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return ActionResponse{}, false
	}
	cached, ok := r.idemCache[idemKey{UserID: claims.UserID, Key: key}]
	if !ok {
		return ActionResponse{}, false
	}
	freshView := r.buildViewLocked(claims)
	cached.View = freshView
	cached.Phase = r.phase
	cached.EventSeq = r.eventSeq
	cached.PublicChatPolicy = freshView.PublicChatPolicy
	return cached, true
}

func (r *Room) storeIdemLocked(claims auth.Claims, key string, resp ActionResponse) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if len(r.idemCache) >= 512 {
		r.idemCache = make(map[idemKey]ActionResponse)
	}
	r.idemCache[idemKey{UserID: claims.UserID, Key: key}] = resp
}

func (r *Room) dispatchSideEffects(notifs []TargetedNotification, updated bool) {
	if updated && r.onRoomUpdated != nil {
		r.onRoomUpdated(r.workspaceID, r.conversationID)
	}
	if len(notifs) > 0 && r.onNotifications != nil {
		r.onNotifications(notifs)
	}
}

func validateTeamSeats(raw []int, playerCount int, requiredSize int) ([]int, error) {
	if len(raw) != requiredSize {
		return nil, fmt.Errorf("%w：本轮任务必须精确选择 %d 名车队成员（当前传入 %d 人）", ErrInvalidAction, requiredSize, len(raw))
	}
	seen := make(map[int]struct{}, len(raw))
	out := make([]int, 0, len(raw))
	for _, s := range raw {
		if s < 1 || s > playerCount {
			return nil, fmt.Errorf("%w：座位号 #%d 无效（合法范围 1..%d）", ErrInvalidAction, s, playerCount)
		}
		if _, dup := seen[s]; dup {
			return nil, fmt.Errorf("%w：车队提名包含重复座位 #%d", ErrInvalidAction, s)
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Ints(out)
	return out, nil
}

func deriveRoleOptions(roles []Role) RoleOptions {
	var o RoleOptions
	for _, r := range roles {
		switch r {
		case RoleMerlin:
			o.Merlin = true
		case RolePercival:
			o.Percival = true
		case RoleAssassin:
			o.Assassin = true
		case RoleMorgana:
			o.Morgana = true
		case RoleMordred:
			o.Mordred = true
		case RoleOberon:
			o.Oberon = true
		}
	}
	return o
}

func phaseChineseLabel(p Phase) string {
	switch p {
	case PhaseLobby:
		return "准备大厅"
	case PhaseTeamProposal:
		return "队长组队提名"
	case PhaseProposalSpeech:
		return "顺时针轮流发言"
	case PhaseTeamVote:
		return "全员车队公投"
	case PhaseQuestExecution:
		return "车队暗投任务卡"
	case PhaseAssassination:
		return "终局刺客拍刀"
	case PhaseGameOver:
		return "对局结算揭秘"
	default:
		return string(p)
	}
}

func voteText(approve bool) string {
	if approve {
		return "赞成发车"
	}
	return "反对发车"
}

func hasMapKey[K comparable, V any](m map[K]V, k K) bool {
	_, ok := m[k]
	return ok
}

func truncateText(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}
