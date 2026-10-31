package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/zzqDeco/teamie-avalon/internal/auth"
	"github.com/zzqDeco/teamie-avalon/internal/game"
)

const (
	// ServerName is reported in MCP initialize responses.
	ServerName = "teamie-avalon"
	// NotificationMethod is the JSON-RPC notification method pushed over MCP SSE.
	// Teamie agent-runtime wraps this into TEAMIE_APP_NOTIFICATION envelopes.
	NotificationMethod = "notifications/avalon_event"
	// MaxNotificationBytes enforces the < 2 KB Fog-of-War notification budget.
	MaxNotificationBytes = 2048
)

type ctxClaimsKey struct{}
type ctxIdemKey struct{}

type sessionMeta struct {
	SessionID      string
	WorkspaceID    string
	ConversationID string
	UserID         string
	CreatedAt      time.Time
}

type playerKey struct {
	WorkspaceID    string
	ConversationID string
	UserID         string
}

type sessionRegistry struct {
	mu       sync.RWMutex
	byID     map[string]sessionMeta
	byPlayer map[playerKey]map[string]struct{}
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{
		byID:     make(map[string]sessionMeta),
		byPlayer: make(map[playerKey]map[string]struct{}),
	}
}

func (r *sessionRegistry) register(claims auth.Claims) string {
	sid := "av-mcp-" + uuid.NewString()
	meta := sessionMeta{
		SessionID:      sid,
		WorkspaceID:    claims.WorkspaceID,
		ConversationID: claims.ConversationID,
		UserID:         claims.UserID,
		CreatedAt:      time.Now().UTC(),
	}
	pk := playerKey{
		WorkspaceID:    claims.WorkspaceID,
		ConversationID: claims.ConversationID,
		UserID:         claims.UserID,
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[sid] = meta
	set, ok := r.byPlayer[pk]
	if !ok {
		set = make(map[string]struct{})
		r.byPlayer[pk] = set
	}
	set[sid] = struct{}{}
	return sid
}

func (r *sessionRegistry) get(sid string) (sessionMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	meta, ok := r.byID[sid]
	return meta, ok
}

func (r *sessionRegistry) delete(sid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	meta, ok := r.byID[sid]
	if !ok {
		return
	}
	delete(r.byID, sid)
	pk := playerKey{
		WorkspaceID:    meta.WorkspaceID,
		ConversationID: meta.ConversationID,
		UserID:         meta.UserID,
	}
	if set, exists := r.byPlayer[pk]; exists {
		delete(set, sid)
		if len(set) == 0 {
			delete(r.byPlayer, pk)
		}
	}
}

func (r *sessionRegistry) sessionsForPlayer(workspaceID, conversationID, userID string) []string {
	pk := playerKey{
		WorkspaceID:    workspaceID,
		ConversationID: conversationID,
		UserID:         userID,
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	set := r.byPlayer[pk]
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for sid := range set {
		out = append(out, sid)
	}
	return out
}

type requestScopedSessionManager struct {
	registry *sessionRegistry
	mcpSrv   *server.MCPServer
	claims   auth.Claims
}

func (m *requestScopedSessionManager) Generate() string {
	return m.registry.register(m.claims)
}

func (m *requestScopedSessionManager) Validate(sessionID string) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, errors.New("missing Mcp-Session-Id")
	}
	if _, ok := m.registry.get(sessionID); !ok {
		// Return isTerminated=true so StreamableHTTPServer responds with HTTP 404 Not Found,
		// triggering automatic session re-handshake in Teamie agent-runtime.
		return true, nil
	}
	return false, nil
}

func (m *requestScopedSessionManager) Terminate(sessionID string) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		m.registry.delete(sessionID)
		m.mcpSrv.UnregisterSession(context.Background(), sessionID)
	}
	return false, nil
}

type sessionResolver struct {
	registry *sessionRegistry
	mcpSrv   *server.MCPServer
}

func (sr *sessionResolver) ResolveSessionIdManager(r *http.Request) server.SessionIdManager {
	claims, _ := claimsFromContext(r.Context())
	return &requestScopedSessionManager{
		registry: sr.registry,
		mcpSrv:   sr.mcpSrv,
		claims:   claims,
	}
}

// Handler serves MCP Streamable HTTP (2025-03-26) with strict HS256 auth,
// stateful session binding, Idempotency-Key propagation, and targeted Fog-of-War notifications.
type Handler struct {
	verifier   *auth.Verifier
	gameMgr    *game.Manager
	mcpSrv     *server.MCPServer
	streamable *server.StreamableHTTPServer
	sessions   *sessionRegistry

	notifCh   chan []game.TargetedNotification
	closeOnce sync.Once
	closed    chan struct{}
	wg        sync.WaitGroup
}

// NewHandler wires the 10 orthogonal Avalon MCP tools onto an authenticated Streamable HTTP handler.
func NewHandler(version string, verifier *auth.Verifier, gameMgr *game.Manager) *Handler {
	if strings.TrimSpace(version) == "" {
		version = "0.1.0"
	}
	instructions := "阿瓦隆圆桌裁判台（The Resistance: Avalon，支持 5–10 人机同桌博弈）。核心纪律：1) 严格遵守每次工具返回与通知中的 publicChatPolicy：仅当 publicChatPolicy.allowed == true（如 PROPOSAL_SPEECH 轮到你持麦，或 ASSASSINATION 红方商议）时才调用 im_send_message 在群聊自然短句口语发言，切忌长篇 Markdown 汇报；2) 持麦发言完毕后必须立即调用 finish_speech 交麦；3) 梅林请把视野伪装在票型逻辑中谨防终局被刺客拍刀；蓝方好人在任务中只能投 success: true；4) 身份与房间唯一由平台 Context Token 自动绑定。"

	mcpSrv := server.NewMCPServer(
		ServerName,
		version,
		server.WithToolCapabilities(false),
		server.WithInstructions(instructions),
	)

	reg := newSessionRegistry()
	resolver := &sessionResolver{
		registry: reg,
		mcpSrv:   mcpSrv,
	}

	h := &Handler{
		verifier: verifier,
		gameMgr:  gameMgr,
		mcpSrv:   mcpSrv,
		sessions: reg,
		notifCh:  make(chan []game.TargetedNotification, 256),
		closed:   make(chan struct{}),
	}

	h.registerTools()

	h.streamable = server.NewStreamableHTTPServer(
		mcpSrv,
		server.WithSessionIdManagerResolver(resolver),
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			if claims, ok := claimsFromContext(r.Context()); ok {
				ctx = context.WithValue(ctx, ctxClaimsKey{}, claims)
			}
			if idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key")); idemKey != "" {
				ctx = context.WithValue(ctx, ctxIdemKey{}, idemKey)
			}
			return ctx
		}),
	)

	h.wg.Add(1)
	go h.notificationLoop()

	return h
}

// Close stops the background notification dispatcher cleanly.
func (h *Handler) Close() {
	h.closeOnce.Do(func() {
		close(h.closed)
	})
	h.wg.Wait()
}

// ServeHTTP enforces HS256 Context Token authentication and Mcp-Session-Id integrity
// before delegating to the MCP Streamable HTTP server.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	claims, err := h.verifier.VerifyAuthorizationHeader(r.Header.Get("Authorization"))
	if err != nil {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]string{
			"error":   "unauthorized",
			"message": err.Error(),
		})
		return
	}

	sessionID := strings.TrimSpace(r.Header.Get(server.HeaderKeySessionID))
	if (r.Method == http.MethodGet || r.Method == http.MethodDelete) && sessionID == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{
			"error":   "missing_mcp_session_id",
			"message": "Mcp-Session-Id header is required for GET and DELETE",
		})
		return
	}

	if sessionID != "" {
		meta, ok := h.sessions.get(sessionID)
		if !ok {
			writeJSONStatus(w, http.StatusNotFound, map[string]string{
				"error":   "mcp_session_not_found",
				"message": "MCP session expired or server restarted; please re-initialize",
			})
			return
		}
		if meta.WorkspaceID != claims.WorkspaceID ||
			meta.ConversationID != claims.ConversationID ||
			meta.UserID != claims.UserID {
			writeJSONStatus(w, http.StatusForbidden, map[string]string{
				"error":   "mcp_session_identity_mismatch",
				"message": "Context Token identity or conversation does not match Mcp-Session-Id",
			})
			return
		}
	}

	ctx := context.WithValue(r.Context(), ctxClaimsKey{}, *claims)
	h.streamable.ServeHTTP(w, r.WithContext(ctx))
}

// EnqueueNotifications schedules targeted Fog-of-War notifications for delivery over MCP SSE.
func (h *Handler) EnqueueNotifications(notifs []game.TargetedNotification) {
	if len(notifs) == 0 {
		return
	}
	copied := make([]game.TargetedNotification, len(notifs))
	copy(copied, notifs)
	select {
	case h.notifCh <- copied:
	case <-h.closed:
	default:
	}
}

func (h *Handler) notificationLoop() {
	defer h.wg.Done()
	for {
		select {
		case <-h.closed:
			return
		case batch := <-h.notifCh:
			time.Sleep(5 * time.Millisecond)
			for _, n := range batch {
				h.deliverOne(n)
			}
		}
	}
}

func (h *Handler) deliverOne(n game.TargetedNotification) {
	sids := h.sessions.sessionsForPlayer(n.WorkspaceID, n.ConversationID, n.TargetUserID)
	if len(sids) == 0 {
		return
	}

	paramsMap, err := payloadToBoundedParams(n.Payload)
	if err != nil {
		return
	}

	for _, sid := range sids {
		if err := h.mcpSrv.SendNotificationToSpecificClient(sid, NotificationMethod, paramsMap); err != nil {
			if errors.Is(err, server.ErrSessionNotFound) {
				h.sessions.delete(sid)
			}
		}
	}
}

func payloadToBoundedParams(p game.NotificationPayload) (map[string]any, error) {
	encode := func(cand game.NotificationPayload) (map[string]any, int, error) {
		raw, err := json.Marshal(cand)
		if err != nil {
			return nil, 0, err
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, 0, err
		}
		envelope := map[string]any{
			"jsonrpc": "2.0",
			"method":  NotificationMethod,
			"params":  m,
		}
		envBytes, err := json.Marshal(envelope)
		if err != nil {
			return nil, 0, err
		}
		return m, len(envBytes), nil
	}

	m, size, err := encode(p)
	if err != nil {
		return nil, err
	}
	if size < MaxNotificationBytes {
		return m, nil
	}

	// Truncate text fields defensively so notification strictly remains < 2 KB.
	if p.PrivateVision != nil {
		vr := []rune(p.PrivateVision.VisionSummary)
		if len(vr) > 90 {
			cp := *p.PrivateVision
			cp.VisionSummary = string(vr[:90]) + "…"
			p.PrivateVision = &cp
		}
	}
	if sr := []rune(p.Summary); len(sr) > 100 {
		p.Summary = string(sr[:100]) + "…"
	}
	if hr := []rune(p.ActionHint); len(hr) > 100 {
		p.ActionHint = string(hr[:100]) + "…"
	}
	m, _, err = encode(p)
	return m, err
}

func claimsFromContext(ctx context.Context) (auth.Claims, bool) {
	c, ok := ctx.Value(ctxClaimsKey{}).(auth.Claims)
	return c, ok
}

func idempotencyKeyFromRequest(ctx context.Context, req mcp.CallToolRequest) string {
	if req.Header != nil {
		if v := strings.TrimSpace(req.Header.Get("Idempotency-Key")); v != "" {
			return v
		}
	}
	if v, ok := ctx.Value(ctxIdemKey{}).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func (h *Handler) registerTools() {
	// 1. get_my_view
	h.mcpSrv.AddTool(
		mcp.NewTool(
			"get_my_view",
			mcp.WithDescription("读取当前群聊阿瓦隆房间状态与你本人的非对称迷雾视角（角色、privateVision、5轮任务轨、流票计数、历轮公开票型、当前麦序与 publicChatPolicy）。只读无副作用。"),
		),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			claims, ok := claimsFromContext(ctx)
			if !ok {
				return mcp.NewToolResultError("missing verified context token"), nil
			}
			view := h.gameMgr.GetView(claims)
			return marshalToolResult(view)
		},
	)

	// 2. join_lobby
	h.mcpSrv.AddTool(
		mcp.NewTool(
			"join_lobby",
			mcp.WithDescription("在 LOBBY 准备阶段入座圆桌（支持 5–10 人）。已入座时重复调用幂等返回当前座位。"),
			mcp.WithString("preset",
				mcp.Description("可选板子预设（仅空桌或1号房主生效）：5p | 6p | 7p_oberon | 7p_mordred | 8p | 9p | 10p"),
				mcp.Enum("5p", "6p", "7p_oberon", "7p_mordred", "8p", "9p", "10p"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			claims, ok := claimsFromContext(ctx)
			if !ok {
				return mcp.NewToolResultError("missing verified context token"), nil
			}
			preset := req.GetString("preset", "")
			idemKey := idempotencyKeyFromRequest(ctx, req)
			room := h.gameMgr.GetOrCreateRoom(claims.WorkspaceID, claims.ConversationID)
			resp, err := room.JoinLobby(claims, preset, idemKey)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return marshalToolResult(resp)
		},
	)

	// 3. leave_lobby
	h.mcpSrv.AddTool(
		mcp.NewTool(
			"leave_lobby",
			mcp.WithDescription("在 LOBBY 准备阶段离开当前座位。"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			claims, ok := claimsFromContext(ctx)
			if !ok {
				return mcp.NewToolResultError("missing verified context token"), nil
			}
			idemKey := idempotencyKeyFromRequest(ctx, req)
			room := h.gameMgr.GetOrCreateRoom(claims.WorkspaceID, claims.ConversationID)
			resp, err := room.LeaveLobby(claims, idemKey)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return marshalToolResult(resp)
		},
	)

	// 4. start_game
	h.mcpSrv.AddTool(
		mcp.NewTool(
			"start_game",
			mcp.WithDescription("当圆桌入座满 5–10 人时发暗牌开启阿瓦隆对局，随机选出首任队长进入第 1 轮 TEAM_PROPOSAL。"),
			mcp.WithString("preset",
				mcp.Description("可选预设板子（1号房主生效）：5p | 6p | 7p_oberon | 7p_mordred | 8p | 9p | 10p"),
				mcp.Enum("5p", "6p", "7p_oberon", "7p_mordred", "8p", "9p", "10p"),
			),
			mcp.WithNumber("phaseTimeoutSec",
				mcp.Description("可选单阶段超时自动推进秒数（5–300，默认 90）。"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			claims, ok := claimsFromContext(ctx)
			if !ok {
				return mcp.NewToolResultError("missing verified context token"), nil
			}
			input := game.LobbyConfigInput{
				Preset:          req.GetString("preset", ""),
				PhaseTimeoutSec: req.GetInt("phaseTimeoutSec", req.GetInt("phase_timeout_sec", 0)),
			}
			idemKey := idempotencyKeyFromRequest(ctx, req)
			room := h.gameMgr.GetOrCreateRoom(claims.WorkspaceID, claims.ConversationID)
			resp, err := room.StartGame(claims, input, idemKey)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return marshalToolResult(resp)
		},
	)

	// 5. propose_team
	h.mcpSrv.AddTool(
		mcp.NewTool(
			"propose_team",
			mcp.WithDescription("在 TEAM_PROPOSAL 阶段由当前队长（leaderSeat）调用，选择本轮任务所需人数的座位号组成车队，随后进入顺时针发言阶段。"),
			mcp.WithArray("teamSeats",
				mcp.Required(),
				mcp.Items(map[string]any{"type": "integer"}),
				mcp.Description("本轮提名上车的座位号数组（如 [1, 3]，人数须严格等于 currentTeamSize 且不重复）。"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			claims, ok := claimsFromContext(ctx)
			if !ok {
				return mcp.NewToolResultError("missing verified context token"), nil
			}
			seats, err := extractIntSliceArg(req, "teamSeats", "team_seats")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			idemKey := idempotencyKeyFromRequest(ctx, req)
			room := h.gameMgr.GetOrCreateRoom(claims.WorkspaceID, claims.ConversationID)
			resp, err := room.ProposeTeam(claims, seats, idemKey)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return marshalToolResult(resp)
		},
	)

	// 6. finish_speech
	h.mcpSrv.AddTool(
		mcp.NewTool(
			"finish_speech",
			mcp.WithDescription("在 PROPOSAL_SPEECH 顺时针轮流发言阶段，仅当前持麦玩家（currentSpeakerSeat）在群聊发送完口语发言后调用，结束发言并交麦（队长末位交麦后开启全员公投）。"),
			mcp.WithString("summary",
				mcp.Description("可选一句话态度摘要（记录至圆桌摘要栏，不超过 100 字）。"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			claims, ok := claimsFromContext(ctx)
			if !ok {
				return mcp.NewToolResultError("missing verified context token"), nil
			}
			summary := req.GetString("summary", "")
			idemKey := idempotencyKeyFromRequest(ctx, req)
			room := h.gameMgr.GetOrCreateRoom(claims.WorkspaceID, claims.ConversationID)
			resp, err := room.FinishSpeech(claims, summary, idemKey)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return marshalToolResult(resp)
		},
	)

	// 7. vote_team
	h.mcpSrv.AddTool(
		mcp.NewTool(
			"vote_team",
			mcp.WithDescription("在 TEAM_VOTE 车队公投阶段暗投赞成（approve=true）或反对（approve=false）；全员投完后公开亮出每座票型。连续 5 次否决红方直接获胜。"),
			mcp.WithBoolean("approve",
				mcp.Required(),
				mcp.Description("true 表示赞成该车队发车，false 表示反对否决。"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			claims, ok := claimsFromContext(ctx)
			if !ok {
				return mcp.NewToolResultError("missing verified context token"), nil
			}
			approve, err := extractRequiredBoolArg(req, "approve")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			idemKey := idempotencyKeyFromRequest(ctx, req)
			room := h.gameMgr.GetOrCreateRoom(claims.WorkspaceID, claims.ConversationID)
			resp, err := room.VoteTeam(claims, approve, idemKey)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return marshalToolResult(resp)
		},
	)

	// 8. submit_quest_card
	h.mcpSrv.AddTool(
		mcp.NewTool(
			"submit_quest_card",
			mcp.WithDescription("在 QUEST_EXECUTION 阶段由入选本轮车队的玩家匿名提交任务卡：蓝方好人必须传 success=true；红方坏人可自由传 true（藏身份）或 false（破坏炸车）。"),
			mcp.WithBoolean("success",
				mcp.Required(),
				mcp.Description("true 为任务成功卡；false 为任务失败卡（仅红方坏人可用）。"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			claims, ok := claimsFromContext(ctx)
			if !ok {
				return mcp.NewToolResultError("missing verified context token"), nil
			}
			success, err := extractRequiredBoolArg(req, "success")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			idemKey := idempotencyKeyFromRequest(ctx, req)
			room := h.gameMgr.GetOrCreateRoom(claims.WorkspaceID, claims.ConversationID)
			resp, err := room.SubmitQuestCard(claims, success, idemKey)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return marshalToolResult(resp)
		},
	)

	// 9. assassinate
	h.mcpSrv.AddTool(
		mcp.NewTool(
			"assassinate",
			mcp.WithDescription("在 ASSASSINATION 终局刺客拍刀阶段，仅由红方刺客（ASSASSIN）调用，指定一名疑似梅林（MERLIN）的座位号进行刺杀；刺中梅林则红方逆转翻盘！"),
			mcp.WithNumber("targetSeat",
				mcp.Required(),
				mcp.Description("刺杀目标座位号（1..N，不可为自己或已知红方同伴）。"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			claims, ok := claimsFromContext(ctx)
			if !ok {
				return mcp.NewToolResultError("missing verified context token"), nil
			}
			targetSeat := req.GetInt("targetSeat", req.GetInt("target_seat", 0))
			if targetSeat <= 0 {
				return mcp.NewToolResultError("targetSeat 必填（1..N）"), nil
			}
			idemKey := idempotencyKeyFromRequest(ctx, req)
			room := h.gameMgr.GetOrCreateRoom(claims.WorkspaceID, claims.ConversationID)
			resp, err := room.Assassinate(claims, targetSeat, idemKey)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return marshalToolResult(resp)
		},
	)

	// 10. reset_lobby
	h.mcpSrv.AddTool(
		mcp.NewTool(
			"reset_lobby",
			mcp.WithDescription("在 GAME_OVER 对局结算后原地重置房间回 LOBBY 准备阶段（保留当前入座玩家），以便在同一群聊直接开启下一局。"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			claims, ok := claimsFromContext(ctx)
			if !ok {
				return mcp.NewToolResultError("missing verified context token"), nil
			}
			idemKey := idempotencyKeyFromRequest(ctx, req)
			room := h.gameMgr.GetOrCreateRoom(claims.WorkspaceID, claims.ConversationID)
			resp, err := room.ResetLobby(claims, idemKey)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return marshalToolResult(resp)
		},
	)
}

func extractIntSliceArg(req mcp.CallToolRequest, keys ...string) ([]int, error) {
	args := req.GetArguments()
	var rawVal any
	found := false
	for _, k := range keys {
		if v, ok := args[k]; ok {
			rawVal = v
			found = true
			break
		}
	}
	if !found || rawVal == nil {
		return nil, errors.New("参数 teamSeats 必填（如 [1, 2]）")
	}

	switch arr := rawVal.(type) {
	case []int:
		return arr, nil
	case []any:
		out := make([]int, 0, len(arr))
		for _, item := range arr {
			switch v := item.(type) {
			case float64:
				out = append(out, int(v))
			case int:
				out = append(out, v)
			case int64:
				out = append(out, int(v))
			case json.Number:
				n, err := v.Int64()
				if err != nil {
					return nil, fmt.Errorf("无效的座位号 %v", item)
				}
				out = append(out, int(n))
			case string:
				n, err := strconv.Atoi(strings.TrimSpace(v))
				if err != nil {
					return nil, fmt.Errorf("无效的座位号 %q", v)
				}
				out = append(out, n)
			default:
				return nil, fmt.Errorf("无效的座位号类型 %T", item)
			}
		}
		return out, nil
	default:
		return nil, errors.New("参数 teamSeats 必须为整数数组（如 [1, 2]）")
	}
}

func extractRequiredBoolArg(req mcp.CallToolRequest, key string) (bool, error) {
	args := req.GetArguments()
	val, ok := args[key]
	if !ok || val == nil {
		return false, fmt.Errorf("布尔参数 %s 必填（true 或 false）", key)
	}
	switch v := val.(type) {
	case bool:
		return v, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1":
			return true, nil
		case "false", "0":
			return false, nil
		}
	}
	return false, fmt.Errorf("参数 %s 必须为布尔值 true 或 false", key)
}

func marshalToolResult(v any) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal response failed: %v", err)), nil
	}
	return mcp.NewToolResultText(string(raw)), nil
}

func writeJSONStatus(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
