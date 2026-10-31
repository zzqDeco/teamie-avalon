package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zzqDeco/teamie-avalon/internal/auth"
	"github.com/zzqDeco/teamie-avalon/internal/game"
	"github.com/zzqDeco/teamie-avalon/internal/httpapi"
)

func TestHTTPServer_Stage2WebUI_AntiSpoofing_And_QuestAnonymity(t *testing.T) {
	const (
		appID        = "33333333-4444-4555-8666-777777777777"
		secret       = "tmsec_http_server_test_secret_0123456789"
		teamieOrigin = "https://120.25.210.202"
	)

	verifier, err := auth.NewVerifier(appID, secret)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	mgr := game.NewManager(game.DefaultTimings())
	appSrv := httpapi.NewServer(httpapi.Config{
		AppID:         appID,
		SigningSecret: secret,
		TeamieOrigin:  teamieOrigin,
		Version:       "0.1.0",
		GitSHA:        "abcdef123456",
		DevSimulator:  false,
	}, verifier, mgr)
	srv := httptest.NewServer(appSrv)
	t.Cleanup(func() {
		srv.Close()
		appSrv.Close()
	})

	// 1. Stage 2 (web_ui) check: GET / returns 200 text/html, no X-Frame-Options, CSP frame-ancestors allows TeamieOrigin
	webResp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	webBody, _ := io.ReadAll(webResp.Body)
	_ = webResp.Body.Close()

	if webResp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", webResp.StatusCode)
	}
	if ct := webResp.Header.Get("Content-Type"); !strings.Contains(strings.ToLower(ct), "text/html") {
		t.Fatalf("GET / Content-Type = %q, want text/html", ct)
	}
	if xfo := webResp.Header.Get("X-Frame-Options"); xfo != "" {
		t.Fatalf("GET / must not set X-Frame-Options, got %q", xfo)
	}
	if csp := webResp.Header.Get("Content-Security-Policy"); csp != "frame-ancestors "+teamieOrigin {
		t.Fatalf("GET / CSP = %q, want %q", csp, "frame-ancestors "+teamieOrigin)
	}
	if !strings.Contains(strings.ToLower(string(webBody)), "<!doctype html") {
		t.Fatalf("GET / body missing <!doctype html: %s", string(webBody))
	}

	// 2. /dev and /api/dev/tokens MUST return 404 when DEV_SIMULATOR=false
	devResp, _ := http.Get(srv.URL + "/dev")
	_ = devResp.Body.Close()
	if devResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected /dev to return 404 in production mode, got %d", devResp.StatusCode)
	}
	devTokResp, _ := http.Post(srv.URL+"/api/dev/tokens", "application/json", strings.NewReader(`{}`))
	_ = devTokResp.Body.Close()
	if devTokResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected /api/dev/tokens to return 404 in production mode, got %d", devTokResp.StatusCode)
	}

	// 3. Unauthenticated /api/view MUST return 401
	unauthView, _ := http.Get(srv.URL + "/api/view")
	_ = unauthView.Body.Close()
	if unauthView.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated /api/view, got %d", unauthView.StatusCode)
	}

	// 4. Anti-spoofing & HTTP action flow: spoofed userId/workspaceId/conversationId in JSON body MUST be ignored!
	mint := func(sub, name string) string {
		now := time.Now().UTC()
		tok, _ := auth.SignToken(secret, auth.Claims{
			AppID:          appID,
			Slug:           "avalon",
			WorkspaceID:    "ws-real",
			ConversationID: "conv-real",
			UserID:         sub,
			DisplayName:    name,
			IssuedAt:       now.Unix(),
			ExpiresAt:      now.Add(time.Hour).Unix(),
		})
		return tok
	}

	tokens := make([]string, 5)
	for i := 0; i < 5; i++ {
		tokens[i] = mint("real-player-"+string(rune('1'+i)), "骑士"+string(rune('1'+i)))
		spoofBody := `{"userId":"spoofed-admin","workspaceId":"spoofed-ws","conversationId":"spoofed-conv","preset":"5p"}`
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/action/join_lobby", strings.NewReader(spoofBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokens[i])
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("join_lobby %d: %v", i+1, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("join_lobby %d status = %d", i+1, resp.StatusCode)
		}
	}

	if _, spoofedExists := mgr.GetRoom("spoofed-ws", "spoofed-conv"); spoofedExists {
		t.Fatal("spoofed workspaceId/conversationId from request body must NEVER create a room")
	}
	room, realExists := mgr.GetRoom("ws-real", "conv-real")
	if !realExists {
		t.Fatal("expected room created under JWT claims (ws-real, conv-real)")
	}

	// Fix roles so Seat 1=MERLIN (Good), Seat 2=MORGANA (Evil)
	room.SetTestDeckOverride([]game.Role{
		game.RoleMerlin,
		game.RoleMorgana,
		game.RolePercival,
		game.RoleServant,
		game.RoleAssassin,
	}, 1)

	postAuth := func(tok, path string, body any) (*http.Response, []byte) {
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp, b
	}

	postAuth(tokens[0], "/api/action/start_game", map[string]any{})
	postAuth(tokens[0], "/api/action/propose_team", map[string]any{"teamSeats": []int{1, 2}})
	for _, s := range []int{2, 3, 4, 5, 1} {
		postAuth(tokens[s-1], "/api/action/finish_speech", map[string]any{"summary": "过"})
	}
	for i := 0; i < 5; i++ {
		postAuth(tokens[i], "/api/action/vote_team", map[string]any{"approve": true})
	}

	// Good player (#1 Merlin) tries to play success=false via HTTP API -> MUST get HTTP 403!
	badResp, _ := postAuth(tokens[0], "/api/action/submit_quest_card", map[string]any{"success": false})
	if badResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected HTTP 403 when Good player submits success=false, got %d", badResp.StatusCode)
	}

	postAuth(tokens[0], "/api/action/submit_quest_card", map[string]any{"success": true})
	postAuth(tokens[1], "/api/action/submit_quest_card", map[string]any{"success": false})

	// Inspect /api/view for every player: verify quest 1 shows (successCount=1, failCount=1) and ZERO card attribution!
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/view", nil)
		req.Header.Set("Authorization", "Bearer "+tokens[i])
		resp, _ := http.DefaultClient.Do(req)
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var pv game.PlayerView
		if err := json.Unmarshal(bodyBytes, &pv); err != nil {
			t.Fatalf("decode PlayerView: %v", err)
		}
		if len(pv.CompletedQuests) != 1 || pv.CompletedQuests[0].SuccessCount != 1 || pv.CompletedQuests[0].FailCount != 1 {
			t.Fatalf("unexpected CompletedQuests for player %d: %+v", i+1, pv.CompletedQuests)
		}
	}
}
