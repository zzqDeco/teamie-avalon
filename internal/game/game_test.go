package game_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zzqDeco/teamie-avalon/internal/auth"
	"github.com/zzqDeco/teamie-avalon/internal/game"
)

func makePlayerClaims(n int) []auth.Claims {
	out := make([]auth.Claims, n)
	for i := 0; i < n; i++ {
		out[i] = auth.Claims{
			WorkspaceID:    "ws-avalon",
			ConversationID: "conv-avalon",
			UserID:         fmt.Sprintf("user-%02d", i+1),
			DisplayName:    fmt.Sprintf("骑士%d号", i+1),
			AppID:          "app-avalon",
			Slug:           "avalon",
		}
	}
	return out
}

func TestAsymmetricFogOfWarVision_AllRoles(t *testing.T) {
	// 10-player full board containing every special role:
	// Seat 1: MERLIN
	// Seat 2: PERCIVAL
	// Seat 3: SERVANT
	// Seat 4: SERVANT
	// Seat 5: SERVANT
	// Seat 6: SERVANT
	// Seat 7: ASSASSIN
	// Seat 8: MORGANA
	// Seat 9: MORDRED
	// Seat 10: OBERON
	roles := []game.Role{
		game.RoleMerlin,
		game.RolePercival,
		game.RoleServant,
		game.RoleServant,
		game.RoleServant,
		game.RoleServant,
		game.RoleAssassin,
		game.RoleMorgana,
		game.RoleMordred,
		game.RoleOberon,
	}

	mgr := game.NewManager(game.DefaultTimings())
	defer mgr.CloseAll()
	room := mgr.GetOrCreateRoom("ws-avalon", "conv-avalon")
	players := makePlayerClaims(10)
	for _, cl := range players {
		if _, err := room.JoinLobby(cl, "", ""); err != nil {
			t.Fatalf("JoinLobby failed: %v", err)
		}
	}
	room.SetTestDeckOverride(roles, 1)
	if _, err := room.StartGame(players[0], game.LobbyConfigInput{}, ""); err != nil {
		t.Fatalf("StartGame failed: %v", err)
	}

	// 1. Assert Loyal Servant (#3) sees ZERO identities anywhere in PlayerView.
	servantView := room.GetView(players[2])
	if servantView.You.Role != game.RoleServant || servantView.You.Alignment != game.AlignmentGood {
		t.Fatalf("expected SERVANT/GOOD, got %+v", servantView.You)
	}
	if pv := servantView.You.PrivateVision; pv == nil ||
		len(pv.VisibleEvilSeats) != 0 ||
		len(pv.MerlinCandidateSeats) != 0 ||
		len(pv.EvilTeammateSeats) != 0 {
		t.Fatalf("SERVANT must have empty private vision lists, got %+v", pv)
	}
	for _, sv := range servantView.Seats {
		if sv.RevealedRole != "" || sv.RevealedAlignment != "" || sv.RevealedRoleName != "" {
			t.Fatalf("seat %d leaked role before GAME_OVER: %+v", sv.Seat, sv)
		}
	}
	rawServantJSON, _ := json.Marshal(servantView)
	for _, forbidden := range []string{"MERLIN", "PERCIVAL", "ASSASSIN", "MORGANA", "MORDRED", "OBERON"} {
		if strings.Contains(string(rawServantJSON), forbidden) {
			t.Fatalf("SERVANT serialized JSON leaked role token %q: %s", forbidden, string(rawServantJSON))
		}
	}

	// 2. Assert Merlin (#1) sees Assassin(#7), Morgana(#8), Oberon(#10), and NEVER sees Mordred(#9)!
	merlinView := room.GetView(players[0])
	wantMerlinEvil := []int{7, 8, 10}
	if !slices.Equal(merlinView.You.PrivateVision.VisibleEvilSeats, wantMerlinEvil) {
		t.Fatalf("Merlin VisibleEvilSeats = %v, want %v (must include Oberon #10 and exclude Mordred #9)",
			merlinView.You.PrivateVision.VisibleEvilSeats, wantMerlinEvil)
	}
	if len(merlinView.You.PrivateVision.EvilTeammateSeats) != 0 || len(merlinView.You.PrivateVision.MerlinCandidateSeats) != 0 {
		t.Fatalf("Merlin must not receive other vision fields: %+v", merlinView.You.PrivateVision)
	}

	// 3. Assert Percival (#2) sees {Merlin(#1), Morgana(#8)} with deterministic anti-refresh order and zero role-order leak.
	percivalView1 := room.GetView(players[1])
	cands := percivalView1.You.PrivateVision.MerlinCandidateSeats
	if len(cands) != 2 || !(slices.Contains(cands, 1) && slices.Contains(cands, 8)) {
		t.Fatalf("Percival MerlinCandidateSeats = %v, want permutation of [1, 8]", cands)
	}
	for i := 0; i < 25; i++ {
		again := room.GetView(players[1]).You.PrivateVision.MerlinCandidateSeats
		if !slices.Equal(cands, again) {
			t.Fatalf("Percival candidate order changed across refreshes: %v vs %v", cands, again)
		}
	}

	// 4. Assert regular Evil (#7 Assassin, #8 Morgana, #9 Mordred) mutually see each other and NEVER see Oberon (#10).
	assassinView := room.GetView(players[6])
	if !slices.Equal(assassinView.You.PrivateVision.EvilTeammateSeats, []int{8, 9}) {
		t.Fatalf("Assassin EvilTeammateSeats = %v, want [8, 9] (must exclude Oberon #10)", assassinView.You.PrivateVision.EvilTeammateSeats)
	}
	morganaView := room.GetView(players[7])
	if !slices.Equal(morganaView.You.PrivateVision.EvilTeammateSeats, []int{7, 9}) {
		t.Fatalf("Morgana EvilTeammateSeats = %v, want [7, 9]", morganaView.You.PrivateVision.EvilTeammateSeats)
	}
	mordredView := room.GetView(players[8])
	if !slices.Equal(mordredView.You.PrivateVision.EvilTeammateSeats, []int{7, 8}) {
		t.Fatalf("Mordred EvilTeammateSeats = %v, want [7, 8]", mordredView.You.PrivateVision.EvilTeammateSeats)
	}

	// 5. Assert Oberon (#10) sees ZERO Evil teammates!
	oberonView := room.GetView(players[9])
	if len(oberonView.You.PrivateVision.EvilTeammateSeats) != 0 ||
		len(oberonView.You.PrivateVision.VisibleEvilSeats) != 0 ||
		len(oberonView.You.PrivateVision.MerlinCandidateSeats) != 0 {
		t.Fatalf("Oberon must see zero teammates, got %+v", oberonView.You.PrivateVision)
	}
}

func TestPercivalCandidates_ZeroRoleOrderLeakage(t *testing.T) {
	opts := game.RoleOptions{Merlin: true, Percival: true, Assassin: true, Morgana: true}
	// Scenario A: Seat 2 is Merlin, Seat 5 is Morgana
	rolesA := map[int]game.Role{
		1: game.RolePercival,
		2: game.RoleMerlin,
		3: game.RoleServant,
		4: game.RoleAssassin,
		5: game.RoleMorgana,
	}
	// Scenario B: Seat 2 is Morgana, Seat 5 is Merlin (roles swapped!)
	rolesB := map[int]game.Role{
		1: game.RolePercival,
		2: game.RoleMorgana,
		3: game.RoleServant,
		4: game.RoleAssassin,
		5: game.RoleMerlin,
	}

	firstIs2 := 0
	firstIs5 := 0
	for i := 0; i < 200; i++ {
		gid := fmt.Sprintf("game-%04d", i)
		va := game.ComputePrivateVision(gid, "percival-sub", 1, rolesA, opts)
		vb := game.ComputePrivateVision(gid, "percival-sub", 1, rolesB, opts)
		// Byte-identical regardless of which seat holds Merlin vs Morgana!
		if !slices.Equal(va.MerlinCandidateSeats, vb.MerlinCandidateSeats) {
			t.Fatalf("role swap leaked order! A=%v B=%v", va.MerlinCandidateSeats, vb.MerlinCandidateSeats)
		}
		if va.MerlinCandidateSeats[0] == 2 {
			firstIs2++
		} else if va.MerlinCandidateSeats[0] == 5 {
			firstIs5++
		}
	}
	if firstIs2 < 60 || firstIs5 < 60 {
		t.Fatalf("expected balanced deterministic shuffle across games, got seat2First=%d seat5First=%d", firstIs2, firstIs5)
	}
}

func TestFullGameplay_GoodCannotFail_AnonymousQuest_AssassinationAndReset(t *testing.T) {
	// 5p board:
	// Seat 1: MERLIN
	// Seat 2: PERCIVAL
	// Seat 3: SERVANT
	// Seat 4: MORGANA
	// Seat 5: ASSASSIN
	roles := []game.Role{
		game.RoleMerlin,
		game.RolePercival,
		game.RoleServant,
		game.RoleMorgana,
		game.RoleAssassin,
	}

	var capturedNotifs []game.TargetedNotification
	mgr := game.NewManager(game.DefaultTimings())
	defer mgr.CloseAll()
	mgr.SetCallbacks(func(ns []game.TargetedNotification) {
		capturedNotifs = append(capturedNotifs, ns...)
	}, nil)

	room := mgr.GetOrCreateRoom("ws-1", "conv-1")
	players := makePlayerClaims(5)
	for _, cl := range players {
		if _, err := room.JoinLobby(cl, "5p", ""); err != nil {
			t.Fatalf("JoinLobby: %v", err)
		}
	}
	room.SetTestDeckOverride(roles, 3) // Initial leader = #3 (SERVANT)
	if _, err := room.StartGame(players[0], game.LobbyConfigInput{}, ""); err != nil {
		t.Fatalf("StartGame: %v", err)
	}

	// Only leader (#3) receives proposal notification; non-leader publicChatPolicy.Allowed == false
	if len(capturedNotifs) != 1 || capturedNotifs[0].TargetUserID != players[2].UserID {
		t.Fatalf("expected only leader #3 to receive start proposal notification, got %+v", capturedNotifs)
	}
	if room.GetView(players[0]).PublicChatPolicy.Allowed {
		t.Fatal("non-leader must have PublicChatPolicy.Allowed=false in TEAM_PROPOSAL")
	}

	// Quest 1 requires 2 players: Leader #3 proposes [#3 (Servant), #4 (Morgana)]
	if _, err := room.ProposeTeam(players[2], []int{4, 3}, "idem-prop-1"); err != nil {
		t.Fatalf("ProposeTeam: %v", err)
	}
	// Verify clockwise speech order starting from leader's left (#4 -> #5 -> #1 -> #2 -> #3 last!)
	v := room.GetView(players[0])
	wantOrder := []int{4, 5, 1, 2, 3}
	if !slices.Equal(v.SpeechOrder, wantOrder) {
		t.Fatalf("SpeechOrder = %v, want %v", v.SpeechOrder, wantOrder)
	}

	// Out-of-turn speaker (#1) must be rejected when #4 holds the mic
	if _, err := room.FinishSpeech(players[0], "抢麦", ""); !errors.Is(err, game.ErrForbiddenRole) {
		t.Fatalf("expected ErrForbiddenRole for out-of-turn speaker, got %v", err)
	}
	for _, seat := range wantOrder {
		spView := room.GetView(players[seat-1])
		if !spView.PublicChatPolicy.Allowed || spView.PublicChatPolicy.ActiveSpeakerSeat != seat {
			t.Fatalf("expected active speaker #%d to have PublicChatPolicy.Allowed=true, got %+v", seat, spView.PublicChatPolicy)
		}
		otherSeat := (seat % 5) + 1
		if room.GetView(players[otherSeat-1]).PublicChatPolicy.Allowed {
			t.Fatalf("non-speaker #%d must have PublicChatPolicy.Allowed=false", otherSeat)
		}
		if _, err := room.FinishSpeech(players[seat-1], fmt.Sprintf("#%d 同意发车", seat), ""); err != nil {
			t.Fatalf("FinishSpeech(#%d): %v", seat, err)
		}
	}

	// Now in TEAM_VOTE: cast 4 approves, 1 reject; verify ballots stay secret before 5th vote!
	for i := 0; i < 4; i++ {
		if _, err := room.VoteTeam(players[i], i != 3, ""); err != nil {
			t.Fatalf("VoteTeam(%d): %v", i+1, err)
		}
	}
	preReveal := room.GetView(players[0])
	if len(preReveal.VoteHistory) != 0 {
		t.Fatal("vote history must not reveal ballots before all players vote")
	}
	if _, err := room.VoteTeam(players[4], true, ""); err != nil {
		t.Fatalf("VoteTeam(5): %v", err)
	}

	// Now in QUEST_EXECUTION with team [#3 (Good), #4 (Evil Morgana)]
	// Hard rule assertion: Good player (#3) attempting success=false MUST be rejected!
	if _, err := room.SubmitQuestCard(players[2], false, ""); !errors.Is(err, game.ErrGoodMustPlaySuccess) {
		t.Fatalf("expected ErrGoodMustPlaySuccess when Good player submits Fail, got %v", err)
	}
	// Good player (#3) submits success=true, Evil (#4) submits success=false (sabotage!)
	if _, err := room.SubmitQuestCard(players[2], true, ""); err != nil {
		t.Fatalf("Good SubmitQuestCard(true): %v", err)
	}
	if _, err := room.SubmitQuestCard(players[3], false, "idem-quest-q1"); err != nil {
		t.Fatalf("Evil SubmitQuestCard(false): %v", err)
	}
	// Idempotency replay of Evil's quest card must return cached response without error
	if _, err := room.SubmitQuestCard(players[3], false, "idem-quest-q1"); err != nil {
		t.Fatalf("idempotent replay of SubmitQuestCard failed: %v", err)
	}

	// Verify Quest 1 failed with (successCount=1, failCount=1) and ZERO mapping to seat #4!
	afterQ1 := room.GetView(players[0])
	if len(afterQ1.CompletedQuests) != 1 {
		t.Fatalf("expected 1 completed quest, got %d", len(afterQ1.CompletedQuests))
	}
	q1 := afterQ1.CompletedQuests[0]
	if q1.Succeeded || q1.SuccessCount != 1 || q1.FailCount != 1 {
		t.Fatalf("unexpected Q1 record: %+v", q1)
	}
	q1Bytes, _ := json.Marshal(q1)
	if strings.Contains(string(q1Bytes), "user-") {
		t.Fatalf("QuestRecord leaked user identity: %s", string(q1Bytes))
	}

	// Run Quests 2, 3, 4 with pure Good teams [1, 2, 3] / [1, 2] / [1, 2, 3] so Blue achieves 3 successes!
	runApprovedGoodQuest := func(team []int) {
		cur := room.GetView(players[0])
		leaderIdx := cur.LeaderSeat - 1
		if _, err := room.ProposeTeam(players[leaderIdx], team, ""); err != nil {
			t.Fatalf("ProposeTeam: %v", err)
		}
		order := room.GetView(players[0]).SpeechOrder
		for _, s := range order {
			if _, err := room.FinishSpeech(players[s-1], "", ""); err != nil {
				t.Fatalf("FinishSpeech: %v", err)
			}
		}
		for i := 0; i < 5; i++ {
			if _, err := room.VoteTeam(players[i], true, ""); err != nil {
				t.Fatalf("VoteTeam: %v", err)
			}
		}
		for _, s := range team {
			if _, err := room.SubmitQuestCard(players[s-1], true, ""); err != nil {
				t.Fatalf("SubmitQuestCard(#%d): %v", s, err)
			}
		}
	}

	runApprovedGoodQuest([]int{1, 2, 3}) // Q2 (size 3) -> Success #1
	runApprovedGoodQuest([]int{1, 2})    // Q3 (size 2) -> Success #2
	runApprovedGoodQuest([]int{1, 2, 3}) // Q4 (size 3) -> Success #3 -> enters ASSASSINATION!

	assassViewGood := room.GetView(players[0])
	if assassViewGood.Phase != game.PhaseAssassination {
		t.Fatalf("expected PhaseAssassination after 3 Good quests, got %s", assassViewGood.Phase)
	}
	if assassViewGood.PublicChatPolicy.Allowed {
		t.Fatal("Good players must stay silent (Allowed=false) during ASSASSINATION")
	}
	assassViewEvil := room.GetView(players[3]) // Morgana (#4)
	if !assassViewEvil.PublicChatPolicy.Allowed {
		t.Fatal("Non-Oberon Evil teammates must have PublicChatPolicy.Allowed=true during ASSASSINATION")
	}

	// Assassin (#5) cannot assassinate Morgana (#4, known evil teammate)
	if _, err := room.Assassinate(players[4], 4, ""); err == nil {
		t.Fatal("expected Assassin targeting known Evil teammate to be rejected")
	}
	// Assassin (#5) snipes Merlin (#1) -> EVIL_WIN_ASSASSINATION!
	if _, err := room.Assassinate(players[4], 1, ""); err != nil {
		t.Fatalf("Assassinate(#1): %v", err)
	}

	over := room.GetView(players[2])
	if over.Phase != game.PhaseGameOver || over.Winner != game.AlignmentEvil || over.WinReason != game.WinReasonEvilAssassinated {
		t.Fatalf("expected EVIL_WIN_ASSASSINATION, got phase=%s winner=%s reason=%s", over.Phase, over.Winner, over.WinReason)
	}
	// All roles revealed in GAME_OVER
	for _, sv := range over.Seats {
		if sv.RevealedRole == "" {
			t.Fatalf("expected seat #%d role revealed in GAME_OVER", sv.Seat)
		}
	}

	// Reset lobby in place for rematch
	if _, err := room.ResetLobby(players[0], "idem-reset"); err != nil {
		t.Fatalf("ResetLobby: %v", err)
	}
	afterReset := room.GetView(players[0])
	if afterReset.Phase != game.PhaseLobby || len(afterReset.Seats) != 5 {
		t.Fatalf("expected LOBBY with 5 retained seats after ResetLobby, got phase=%s seats=%d", afterReset.Phase, len(afterReset.Seats))
	}
}

func TestSevenPlayerProtectedQuest4_And_FiveRejectsEvilWin(t *testing.T) {
	roles := []game.Role{
		game.RoleMerlin,
		game.RolePercival,
		game.RoleServant,
		game.RoleServant,
		game.RoleMorgana,
		game.RoleAssassin,
		game.RoleOberon,
	}

	mgr := game.NewManager(game.DefaultTimings())
	defer mgr.CloseAll()
	room := mgr.GetOrCreateRoom("ws-7p", "conv-7p")
	players := makePlayerClaims(7)
	for _, cl := range players {
		_, _ = room.JoinLobby(cl, "7p_oberon", "")
	}
	room.SetTestDeckOverride(roles, 1)
	if _, err := room.StartGame(players[0], game.LobbyConfigInput{}, ""); err != nil {
		t.Fatalf("StartGame 7p: %v", err)
	}

	passProposalAndVote := func(team []int, approveCount int) {
		cur := room.GetView(players[0])
		_, err := room.ProposeTeam(players[cur.LeaderSeat-1], team, "")
		if err != nil {
			t.Fatalf("ProposeTeam(%v): %v", team, err)
		}
		for _, s := range room.GetView(players[0]).SpeechOrder {
			_, _ = room.FinishSpeech(players[s-1], "", "")
		}
		for i := 0; i < 7; i++ {
			_, _ = room.VoteTeam(players[i], i < approveCount, "")
		}
	}

	// Q1 (size 2): Good wins [1, 2]
	passProposalAndVote([]int{1, 2}, 5)
	_, _ = room.SubmitQuestCard(players[0], true, "")
	_, _ = room.SubmitQuestCard(players[1], true, "")

	// Q2 (size 3): Evil sabotages [1, 2, 5(Morgana)] -> 1 fail breaks Q2
	passProposalAndVote([]int{1, 2, 5}, 5)
	_, _ = room.SubmitQuestCard(players[0], true, "")
	_, _ = room.SubmitQuestCard(players[1], true, "")
	_, _ = room.SubmitQuestCard(players[4], false, "")
	if room.GetView(players[0]).FailQuestCount != 1 {
		t.Fatal("expected Q2 to fail with 1 fail card")
	}

	// Q3 (size 3): Good wins [1, 2, 3]
	passProposalAndVote([]int{1, 2, 3}, 5)
	_, _ = room.SubmitQuestCard(players[0], true, "")
	_, _ = room.SubmitQuestCard(players[1], true, "")
	_, _ = room.SubmitQuestCard(players[2], true, "")

	// Q4 (size 4*, protected! Needs >= 2 fails): Team [1, 2, 3, 5(Morgana)] with ONLY 1 Fail MUST SUCCEED!
	q4View := room.GetView(players[0])
	if !q4View.QuestTrack[3].TwoFailsRequired || q4View.QuestTrack[3].FailsRequired != 2 {
		t.Fatalf("7p Q4 must require 2 fails, got %+v", q4View.QuestTrack[3])
	}
	passProposalAndVote([]int{1, 2, 3, 5}, 5)
	_, _ = room.SubmitQuestCard(players[0], true, "")
	_, _ = room.SubmitQuestCard(players[1], true, "")
	_, _ = room.SubmitQuestCard(players[2], true, "")
	_, _ = room.SubmitQuestCard(players[4], false, "") // 1 fail on protected Q4!

	afterQ4 := room.GetView(players[0])
	if afterQ4.SuccessQuestCount != 3 || afterQ4.Phase != game.PhaseAssassination {
		t.Fatalf("expected Q4 with 1 fail in 7p to SUCCEED (3rd success -> ASSASSINATION), got success=%d phase=%s",
			afterQ4.SuccessQuestCount, afterQ4.Phase)
	}

	// Assassin (#6) mistakenly shoots Oberon (#7, unknown to Assassin) -> misses Merlin -> GOOD_WIN!
	if _, err := room.Assassinate(players[5], 7, ""); err != nil {
		t.Fatalf("Assassin shooting Oberon should be allowed and miss Merlin: %v", err)
	}
	if room.GetView(players[0]).WinReason != game.WinReasonGood {
		t.Fatalf("expected GOOD_WIN when Assassin misses Merlin, got %s", room.GetView(players[0]).WinReason)
	}

	// Rematch to test 5 consecutive rejected proposals -> EVIL_WIN_FIVE_REJECTS
	_, _ = room.ResetLobby(players[0], "")
	room.SetTestDeckOverride(roles, 1)
	_, _ = room.StartGame(players[0], game.LobbyConfigInput{}, "")
	for attempt := 1; attempt <= 5; attempt++ {
		passProposalAndVote([]int{1, 2}, 3) // 3 approves out of 7 -> rejected!
	}
	endView := room.GetView(players[0])
	if endView.Phase != game.PhaseGameOver || endView.WinReason != game.WinReasonEvilFiveRejects {
		t.Fatalf("expected EVIL_WIN_FIVE_REJECTS after 5 rejected proposals, got phase=%s reason=%s", endView.Phase, endView.WinReason)
	}
}

func TestPhaseTimeoutAutoAdvance(t *testing.T) {
	fast := game.Timings{
		ProposalTimeout:      25 * time.Millisecond,
		SpeakerTurnTimeout:   25 * time.Millisecond,
		VoteTimeout:          25 * time.Millisecond,
		QuestTimeout:         25 * time.Millisecond,
		AssassinationTimeout: 25 * time.Millisecond,
	}
	mgr := game.NewManager(fast)
	defer mgr.CloseAll()

	room := mgr.GetOrCreateRoom("ws-timeout", "conv-timeout")
	players := makePlayerClaims(5)
	for _, cl := range players {
		_, _ = room.JoinLobby(cl, "5p", "")
	}
	_, _ = room.StartGame(players[0], game.LobbyConfigInput{}, "")

	// Wait for auto-proposal + 5 auto-speeches + 5 auto-rejects -> EVIL_WIN_FIVE_REJECTS without stalling!
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if room.GetView(players[0]).Phase == game.PhaseGameOver {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected room to auto-advance to GAME_OVER on timeouts, stuck at %s", room.GetView(players[0]).Phase)
}
