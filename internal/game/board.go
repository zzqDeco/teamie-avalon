package game

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

const (
	MinPlayers = 5
	MaxPlayers = 10

	Preset5P        = "5p"
	Preset6P        = "6p"
	Preset7POberon  = "7p_oberon"
	Preset7PMordred = "7p_mordred"
	Preset8P        = "8p"
	Preset9P        = "9p"
	Preset10P       = "10p"
	PresetCustom    = "custom"
)

// FactionCounts returns the canonical (goodCount, evilCount) for 5..10 players.
func FactionCounts(playerCount int) (good int, evil int, ok bool) {
	switch playerCount {
	case 5:
		return 3, 2, true
	case 6:
		return 4, 2, true
	case 7:
		return 4, 3, true
	case 8:
		return 5, 3, true
	case 9:
		return 6, 3, true
	case 10:
		return 6, 4, true
	default:
		return 0, 0, false
	}
}

// QuestMatrix returns the 5 quest team sizes and required fail counts for 5..10 players.
// In 7–10 player games, Quest 4 is a protected quest requiring at least 2 Fail cards.
func QuestMatrix(playerCount int) ([]QuestSpec, bool) {
	var sizes [5]int
	switch playerCount {
	case 5:
		sizes = [5]int{2, 3, 2, 3, 3}
	case 6:
		sizes = [5]int{2, 3, 4, 3, 4}
	case 7:
		sizes = [5]int{2, 3, 3, 4, 4}
	case 8, 9, 10:
		sizes = [5]int{3, 4, 4, 5, 5}
	default:
		return nil, false
	}

	out := make([]QuestSpec, 5)
	for i := 0; i < 5; i++ {
		qNum := i + 1
		fails := 1
		if playerCount >= 7 && qNum == 4 {
			fails = 2
		}
		out[i] = QuestSpec{
			QuestNumber:      qNum,
			TeamSize:         sizes[i],
			FailsRequired:    fails,
			TwoFailsRequired: fails >= 2,
		}
	}
	return out, true
}

// PresetSpec resolves a preset name into targetPlayerCount and recommended RoleOptions.
func PresetSpec(raw string) (preset string, targetCount int, opts RoleOptions, ok bool) {
	norm := strings.ToLower(strings.TrimSpace(raw))
	switch norm {
	case "5p", "5p_standard", "standard_5p":
		return Preset5P, 5, RoleOptions{
			Merlin: true, Percival: true, Assassin: true, Morgana: true,
		}, true
	case "6p", "6p_standard", "standard_6p":
		return Preset6P, 6, RoleOptions{
			Merlin: true, Percival: true, Assassin: true, Morgana: true,
		}, true
	case "7p", "7p_oberon", "7p_standard":
		return Preset7POberon, 7, RoleOptions{
			Merlin: true, Percival: true, Assassin: true, Morgana: true, Oberon: true,
		}, true
	case "7p_mordred":
		return Preset7PMordred, 7, RoleOptions{
			Merlin: true, Percival: true, Assassin: true, Morgana: true, Mordred: true,
		}, true
	case "8p", "8p_standard":
		return Preset8P, 8, RoleOptions{
			Merlin: true, Percival: true, Assassin: true, Morgana: true, Mordred: true,
		}, true
	case "9p", "9p_standard":
		return Preset9P, 9, RoleOptions{
			Merlin: true, Percival: true, Assassin: true, Morgana: true, Mordred: true,
		}, true
	case "10p", "10p_standard", "10p_full":
		return Preset10P, 10, RoleOptions{
			Merlin: true, Percival: true, Assassin: true, Morgana: true, Mordred: true, Oberon: true,
		}, true
	default:
		return "", 0, RoleOptions{}, false
	}
}

// DefaultPresetForCount returns the standard preset for a given player count 5..10.
func DefaultPresetForCount(count int) (string, RoleOptions) {
	switch count {
	case 6:
		_, _, o, _ := PresetSpec(Preset6P)
		return Preset6P, o
	case 7:
		_, _, o, _ := PresetSpec(Preset7POberon)
		return Preset7POberon, o
	case 8:
		_, _, o, _ := PresetSpec(Preset8P)
		return Preset8P, o
	case 9:
		_, _, o, _ := PresetSpec(Preset9P)
		return Preset9P, o
	case 10:
		_, _, o, _ := PresetSpec(Preset10P)
		return Preset10P, o
	default:
		_, _, o, _ := PresetSpec(Preset5P)
		return Preset5P, o
	}
}

// BuildDeck validates RoleOptions against playerCount and returns the exact unshuffled card deck.
func BuildDeck(playerCount int, opts RoleOptions) ([]Role, error) {
	goodCount, evilCount, ok := FactionCounts(playerCount)
	if !ok {
		return nil, fmt.Errorf("%w：阿瓦隆仅支持 %d–%d 人对局（当前 %d 人）", ErrInvalidAction, MinPlayers, MaxPlayers, playerCount)
	}

	if opts.Assassin && !opts.Merlin {
		return nil, fmt.Errorf("%w：启用刺客（Assassin）时必须同时启用梅林（Merlin）", ErrInvalidAction)
	}
	if opts.Morgana && (!opts.Merlin || !opts.Percival) {
		return nil, fmt.Errorf("%w：启用莫甘娜（Morgana）时必须同时启用梅林（Merlin）与派西维尔（Percival）", ErrInvalidAction)
	}

	goodRoles := make([]Role, 0, goodCount)
	if opts.Merlin {
		goodRoles = append(goodRoles, RoleMerlin)
	}
	if opts.Percival {
		goodRoles = append(goodRoles, RolePercival)
	}
	if len(goodRoles) > goodCount {
		return nil, fmt.Errorf("%w：蓝方特殊角色数量（%d）超过 %d 人局蓝方席位上限（%d）", ErrInvalidAction, len(goodRoles), playerCount, goodCount)
	}
	for len(goodRoles) < goodCount {
		goodRoles = append(goodRoles, RoleServant)
	}

	evilRoles := make([]Role, 0, evilCount)
	if opts.Assassin {
		evilRoles = append(evilRoles, RoleAssassin)
	}
	if opts.Morgana {
		evilRoles = append(evilRoles, RoleMorgana)
	}
	if opts.Mordred {
		evilRoles = append(evilRoles, RoleMordred)
	}
	if opts.Oberon {
		evilRoles = append(evilRoles, RoleOberon)
	}
	if len(evilRoles) > evilCount {
		return nil, fmt.Errorf("%w：当前 %d 人局仅有 %d 个红方席位，但已启用 %d 个红方特殊角色，请增加人数或关闭部分红方神职", ErrInvalidAction, playerCount, evilCount, len(evilRoles))
	}
	for len(evilRoles) < evilCount {
		evilRoles = append(evilRoles, RoleMinion)
	}

	deck := make([]Role, 0, playerCount)
	deck = append(deck, goodRoles...)
	deck = append(deck, evilRoles...)
	return deck, nil
}

// RoleAlignment returns GOOD or EVIL for a Role.
func RoleAlignment(r Role) Alignment {
	switch r {
	case RoleMerlin, RolePercival, RoleServant:
		return AlignmentGood
	default:
		return AlignmentEvil
	}
}

// RoleDisplayName returns the localized Chinese character name.
func RoleDisplayName(r Role) string {
	switch r {
	case RoleMerlin:
		return "梅林 (Merlin)"
	case RolePercival:
		return "派西维尔 (Percival)"
	case RoleServant:
		return "亚瑟忠臣 (Loyal Servant)"
	case RoleAssassin:
		return "刺客 (Assassin)"
	case RoleMorgana:
		return "莫甘娜 (Morgana)"
	case RoleMordred:
		return "莫德雷德 (Mordred)"
	case RoleOberon:
		return "奥伯伦 (Oberon)"
	case RoleMinion:
		return "莫德雷德爪牙 (Minion)"
	default:
		return string(r)
	}
}

// ComputePrivateVision computes the byte-exact asymmetric Fog-of-War vision for viewerSeat/viewerSub.
// Crucially:
//   - MERLIN sees all EVIL seats EXCEPT MORDRED (includes OBERON, without exposing specific evil roles).
//   - PERCIVAL sees [MERLIN, MORGANA] deterministically shuffled by (gameID, viewerSub) — zero role-order leak.
//   - SERVANT sees no extra seats.
//   - ASSASSIN / MORGANA / MORDRED / MINION see other EVIL seats EXCEPT OBERON and self.
//   - OBERON sees no evil teammates, and no evil teammates see OBERON.
func ComputePrivateVision(gameID, viewerSub string, viewerSeat int, seatRoles map[int]Role, opts RoleOptions) PrivateVision {
	myRole := seatRoles[viewerSeat]
	switch myRole {
	case RoleMerlin:
		evilSeats := make([]int, 0)
		for seat, r := range seatRoles {
			if RoleAlignment(r) == AlignmentEvil && r != RoleMordred {
				evilSeats = append(evilSeats, seat)
			}
		}
		sort.Ints(evilSeats)
		blindNote := ""
		if opts.Mordred {
			blindNote = "（注意：本局包含莫德雷德，他已隐匿在你的视野之外！）"
		}
		if len(evilSeats) == 0 {
			return PrivateVision{
				VisionSummary: "你是梅林：当前未探知可见红方座位" + blindNote + "。发言请务必隐藏上帝视角，谨防终局被刺客拍刀！",
			}
		}
		return PrivateVision{
			VisibleEvilSeats: evilSeats,
			VisionSummary:    fmt.Sprintf("你是梅林：可见红方座位 %v%s。切记把信息包装成票型与车队逻辑，绝不可直白暴露上帝视角以免终局被刺客拍刀！", formatSeatList(evilSeats), blindNote),
		}

	case RolePercival:
		candidates := make([]int, 0, 2)
		for seat, r := range seatRoles {
			if r == RoleMerlin || r == RoleMorgana {
				candidates = append(candidates, seat)
			}
		}
		shuffled := DeterministicSeatOrder(gameID, viewerSub, candidates)
		if len(shuffled) == 0 {
			return PrivateVision{
				VisionSummary: "你是派西维尔：本局未配置梅林/莫甘娜候选视野，请通过发言协助蓝方带队。",
			}
		}
		if opts.Morgana && len(shuffled) > 1 {
			return PrivateVision{
				MerlinCandidateSeats: shuffled,
				VisionSummary:        fmt.Sprintf("你是派西维尔：真假梅林候选座位为 %v（顺序已按本局密钥随机打乱，其中一位是真梅林，另一位是伪装梅林的莫甘娜）。请通过发言与票型分辨真梅林并替他挡刀！", formatSeatList(shuffled)),
			}
		}
		return PrivateVision{
			MerlinCandidateSeats: shuffled,
			VisionSummary:        fmt.Sprintf("你是派西维尔：梅林候选座位为 %v。请在公屏掩护梅林，误导刺客拍刀你自己！", formatSeatList(shuffled)),
		}

	case RoleServant:
		return PrivateVision{
			VisionSummary: "你是亚瑟忠臣（蓝方好人）：无初始特殊视野。请紧盯每轮车队提名与公投票型找坏人，任务上车必须投任务成功。",
		}

	case RoleOberon:
		return PrivateVision{
			VisionSummary: "你是奥伯伦（红方孤狼）：你看不到红方队友，红方队友也看不到你（但梅林能看到你）。请观察盘面制造混乱或暗中炸车协助红方获胜！",
		}

	case RoleAssassin, RoleMorgana, RoleMordred, RoleMinion:
		teammates := make([]int, 0)
		for seat, r := range seatRoles {
			if seat == viewerSeat {
				continue
			}
			if RoleAlignment(r) == AlignmentEvil && r != RoleOberon {
				teammates = append(teammates, seat)
			}
		}
		sort.Ints(teammates)
		oberonNote := ""
		if opts.Oberon {
			oberonNote = "（本局另有孤狼奥伯伦，彼此不可见）"
		}
		roleTip := "请与红方同伴默契打配合骗取上车炸车。"
		switch myRole {
		case RoleAssassin:
			roleTip = "你是红方刺客：除配合炸车外，务必全程留意哪位蓝方玩家发言暗藏上帝视角，若蓝方先成 3 局，由你终局拍刀刺杀梅林翻盘！"
		case RoleMorgana:
			roleTip = "你是莫甘娜：你在派西维尔眼中与真梅林并列为候选，请模仿好人带队逻辑迷惑派西维尔！"
		case RoleMordred:
			roleTip = "你是莫德雷德：梅林看不到你的红方身份！可大胆伪装铁好人骗取梅林与车队信任。"
		}
		if len(teammates) == 0 {
			return PrivateVision{
				VisionSummary: fmt.Sprintf("当前无可见红方同伴%s。%s", oberonNote, roleTip),
			}
		}
		return PrivateVision{
			EvilTeammateSeats: teammates,
			VisionSummary:     fmt.Sprintf("已知红方同伴座位：%v%s。%s", formatSeatList(teammates), oberonNote, roleTip),
		}

	default:
		return PrivateVision{VisionSummary: "未知角色"}
	}
}

// DeterministicSeatOrder orders a set of seat numbers strictly using SHA-256(gameID, viewerSub, seat).
// Because the input contains ONLY seat numbers (never Role values), swapping MERLIN and MORGANA
// between the same two seats in the same game produces the exact same output slice byte-for-byte.
func DeterministicSeatOrder(gameID, viewerSub string, seats []int) []int {
	if len(seats) <= 1 {
		out := make([]int, len(seats))
		copy(out, seats)
		return out
	}
	type keyedSeat struct {
		seat int
		hash [32]byte
	}
	items := make([]keyedSeat, len(seats))
	for i, s := range seats {
		msg := fmt.Sprintf("avalon-percival-v1:%s:%s:%d", gameID, viewerSub, s)
		items[i] = keyedSeat{
			seat: s,
			hash: sha256.Sum256([]byte(msg)),
		}
	}
	sort.Slice(items, func(i, j int) bool {
		cmp := bytes.Compare(items[i].hash[:], items[j].hash[:])
		if cmp != 0 {
			return cmp < 0
		}
		return items[i].seat < items[j].seat
	})
	out := make([]int, len(items))
	for i, it := range items {
		out[i] = it.seat
	}
	return out
}

// CryptoShuffleRoles shuffles roles in-place using crypto/rand.
func CryptoShuffleRoles(roles []Role) {
	for i := len(roles) - 1; i > 0; i-- {
		j := cryptoRandIntn(i + 1)
		roles[i], roles[j] = roles[j], roles[i]
	}
}

func cryptoRandIntn(n int) int {
	if n <= 1 {
		return 0
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	return int(binary.LittleEndian.Uint64(b[:]) % uint64(n))
}

func formatSeatList(seats []int) string {
	parts := make([]string, len(seats))
	for i, s := range seats {
		parts[i] = fmt.Sprintf("#%d", s)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
