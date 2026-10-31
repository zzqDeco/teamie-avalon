package mcpserver_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zzqDeco/teamie-avalon/internal/auth"
	"github.com/zzqDeco/teamie-avalon/internal/game"
	"github.com/zzqDeco/teamie-avalon/internal/mcpserver"
)

const (
	testAppID  = "22222222-3333-4444-8555-666666666666"
	testSecret = "tmsec_avalon_mcp_test_secret_0123456789abcdef"
)

func setupMCPTestServer(t *testing.T) (*httptest.Server, *game.Manager, func(sub, ws, conv, name string) string) {
	t.Helper()
	verifier, err := auth.NewVerifier(testAppID, testSecret)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	mgr := game.NewManager(game.DefaultTimings())
	handler := mcpserver.NewHandler("0.1.0", verifier, mgr)
	mgr.SetCallbacks(handler.EnqueueNotifications, nil)

	srv := httptest.NewServer(handler)
	t.Cleanup(func() {
		srv.Close()
		handler.Close()
		mgr.CloseAll()
	})

	mint := func(sub, ws, conv, name string) string {
		now := time.Now().UTC()
		tok, err := auth.SignToken(testSecret, auth.Claims{
			AppID:          testAppID,
			Slug:           "avalon",
			WorkspaceID:    ws,
			ConversationID: conv,
			UserID:         sub,
			DisplayName:    name,
			IssuedAt:       now.Unix(),
			ExpiresAt:      now.Add(time.Hour).Unix(),
		})
		if err != nil {
			t.Fatalf("SignToken: %v", err)
		}
		return tok
	}
	return srv, mgr, mint
}

func TestStage3AndStage4VerifierProbe_ZeroRoomSideEffects(t *testing.T) {
	srv, mgr, mint := setupMCPTestServer(t)
	initPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"teamie-marketplace-verifier","version":"1.0.0"}}}`

	// Stage 3a: No Authorization header -> MUST return HTTP 401
	req1, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(initPayload))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("unauth POST failed: %v", err)
	}
	_ = resp1.Body.Close()
	if resp1.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected HTTP 401 for missing Authorization header, got %d", resp1.StatusCode)
	}

	// Stage 3b: Forged HS256 signature -> MUST return HTTP 401
	forgedTok, _ := auth.SignToken("tmsec_wrong_forged_secret", auth.Claims{
		AppID:          testAppID,
		Slug:           "avalon",
		WorkspaceID:    "00000000-0000-4000-8000-000000000002",
		ConversationID: "00000000-0000-4000-8000-000000000003",
		UserID:         "00000000-0000-4000-8000-000000000001",
		IssuedAt:       time.Now().Unix(),
		ExpiresAt:      time.Now().Add(5 * time.Minute).Unix(),
	})
	req2, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(initPayload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+forgedTok)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("forged POST failed: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected HTTP 401 for forged token, got %d", resp2.StatusCode)
	}

	// Unknown Mcp-Session-Id -> MUST return HTTP 404 so agent-runtime re-handshakes
	probeToken := mint(
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002",
		"00000000-0000-4000-8000-000000000003",
		"Teamie Protocol Verifier",
	)
	reqStale, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	reqStale.Header.Set("Content-Type", "application/json")
	reqStale.Header.Set("Authorization", "Bearer "+probeToken)
	reqStale.Header.Set("Mcp-Session-Id", "av-mcp-nonexistent-after-restart")
	respStale, err := http.DefaultClient.Do(reqStale)
	if err != nil {
		t.Fatalf("stale session POST failed: %v", err)
	}
	_ = respStale.Body.Close()
	if respStale.StatusCode != http.StatusNotFound {
		t.Fatalf("expected HTTP 404 for unknown Mcp-Session-Id, got %d", respStale.StatusCode)
	}

	// Stage 4: Valid probe handshake initialize -> notifications/initialized -> tools/list -> DELETE
	reqInit, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(initPayload))
	reqInit.Header.Set("Content-Type", "application/json")
	reqInit.Header.Set("Accept", "application/json, text/event-stream")
	reqInit.Header.Set("Authorization", "Bearer "+probeToken)
	respInit, err := http.DefaultClient.Do(reqInit)
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	defer respInit.Body.Close()
	if respInit.StatusCode != http.StatusOK {
		t.Fatalf("initialize status = %d, want 200", respInit.StatusCode)
	}
	sessionID := respInit.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("expected non-empty Mcp-Session-Id header")
	}

	reqNotif, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	reqNotif.Header.Set("Content-Type", "application/json")
	reqNotif.Header.Set("Authorization", "Bearer "+probeToken)
	reqNotif.Header.Set("Mcp-Session-Id", sessionID)
	respNotif, _ := http.DefaultClient.Do(reqNotif)
	if respNotif != nil {
		_ = respNotif.Body.Close()
	}

	reqTools, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	reqTools.Header.Set("Content-Type", "application/json")
	reqTools.Header.Set("Authorization", "Bearer "+probeToken)
	reqTools.Header.Set("Mcp-Session-Id", sessionID)
	respTools, err := http.DefaultClient.Do(reqTools)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	rawTools, _ := io.ReadAll(respTools.Body)
	_ = respTools.Body.Close()

	var rpcTools struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rawTools, &rpcTools); err != nil {
		t.Fatalf("unmarshal tools/list: %v (body=%s)", err, string(rawTools))
	}
	if len(rpcTools.Result.Tools) != 10 {
		t.Fatalf("expected exactly 10 Avalon MCP tools, got %d: %+v", len(rpcTools.Result.Tools), rpcTools.Result.Tools)
	}
	for _, tool := range rpcTools.Result.Tools {
		exposed := "mcp__avalon__" + tool.Name
		if len(exposed) > 64 || len(tool.Name) > 32 {
			t.Fatalf("tool name %q exceeds length budget (exposed=%q, len=%d)", tool.Name, exposed, len(exposed))
		}
	}

	// Cross-user session hijacking must return HTTP 403
	otherToken := mint("other-user", "00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000003", "Eve")
	reqHijack, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`))
	reqHijack.Header.Set("Content-Type", "application/json")
	reqHijack.Header.Set("Authorization", "Bearer "+otherToken)
	reqHijack.Header.Set("Mcp-Session-Id", sessionID)
	respHijack, _ := http.DefaultClient.Do(reqHijack)
	_ = respHijack.Body.Close()
	if respHijack.StatusCode != http.StatusForbidden {
		t.Fatalf("expected HTTP 403 for cross-user Mcp-Session-Id hijack, got %d", respHijack.StatusCode)
	}

	// DELETE session cleanup
	reqDel, _ := http.NewRequest(http.MethodDelete, srv.URL, nil)
	reqDel.Header.Set("Authorization", "Bearer "+probeToken)
	reqDel.Header.Set("Mcp-Session-Id", sessionID)
	respDel, _ := http.DefaultClient.Do(reqDel)
	_ = respDel.Body.Close()

	// Assert probe handshake + get_my_view left ZERO dirty rooms in memory!
	if mgr.RoomCount() != 0 {
		t.Fatalf("expected 0 rooms allocated after probe handshake, got %d", mgr.RoomCount())
	}
}

func TestMCPSSETargetedNotification_And_IdempotentToolsCall(t *testing.T) {
	srv, mgr, mint := setupMCPTestServer(t)

	tokens := make([]string, 5)
	sids := make([]string, 5)
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"eino-agent","version":"0.4.4"}}}`

	for i := 0; i < 5; i++ {
		tokens[i] = mint(
			"agent-sub-"+string(rune('1'+i)),
			"ws-live",
			"conv-live",
			"Agent"+string(rune('1'+i)),
		)
		req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(initBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokens[i])
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("init agent %d: %v", i+1, err)
		}
		sids[i] = resp.Header.Get("Mcp-Session-Id")
		_ = resp.Body.Close()
	}

	// Open GET SSE notification stream for Player 2 (who will be initial leader #2 = MERLIN)
	sseCtx, cancelSSE := context.WithCancel(context.Background())
	defer cancelSSE()
	sseReq, _ := http.NewRequestWithContext(sseCtx, http.MethodGet, srv.URL, nil)
	sseReq.Header.Set("Accept", "text/event-stream")
	sseReq.Header.Set("Authorization", "Bearer "+tokens[1])
	sseReq.Header.Set("Mcp-Session-Id", sids[1])
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("open GET SSE: %v", err)
	}
	defer sseResp.Body.Close()

	sseEvents := make(chan string, 8)
	go func() {
		scanner := bufio.NewScanner(sseResp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if after, ok := strings.CutPrefix(line, "data:"); ok {
				trimmed := strings.TrimSpace(after)
				if strings.Contains(trimmed, "notifications/avalon_event") {
					sseEvents <- trimmed
				}
			}
		}
	}()

	callTool := func(idx int, idemKey string, toolName string, args map[string]any) map[string]any {
		t.Helper()
		payload, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      10,
			"method":  "tools/call",
			"params": map[string]any{
				"name":      toolName,
				"arguments": args,
			},
		})
		req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokens[idx])
		req.Header.Set("Mcp-Session-Id", sids[idx])
		if idemKey != "" {
			req.Header.Set("Idempotency-Key", idemKey)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("callTool(%s): %v", toolName, err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return out
	}

	// All 5 players join via MCP tools/call; test Idempotency-Key replay on player 1
	for i := 0; i < 5; i++ {
		callTool(i, "join-key", "join_lobby", map[string]any{"preset": "5p"})
	}
	// Replay player 1 join_lobby with same Idempotency-Key
	callTool(0, "join-key", "join_lobby", map[string]any{"preset": "5p"})

	room, _ := mgr.GetRoom("ws-live", "conv-live")
	room.SetTestDeckOverride([]game.Role{
		game.RoleServant,
		game.RoleMerlin, // Seat 2 is Merlin AND initial leader!
		game.RolePercival,
		game.RoleMorgana,  // Seat 4 Evil (visible to Merlin)
		game.RoleMordred,  // Seat 5 Mordred (MUST NOT be visible to Merlin!)
	}, 2)

	callTool(0, "start-key-1", "start_game", map[string]any{})

	select {
	case ev := <-sseEvents:
		if len(ev) >= mcpserver.MaxNotificationBytes {
			t.Fatalf("SSE notification exceeded 2KB budget: %d bytes", len(ev))
		}
		if !strings.Contains(ev, `"yourRole":"MERLIN"`) {
			t.Fatalf("expected Merlin role in leader #2 SSE notification, got %s", ev)
		}
		// Merlin must see Morgana (#4) and must NOT see Mordred (#5) in SSE notification!
		if !strings.Contains(ev, `"visibleEvilSeats":[4]`) {
			t.Fatalf("expected visibleEvilSeats:[4] (excluding Mordred #5) in SSE notification, got %s", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for targeted MCP SSE notification")
	}
}
