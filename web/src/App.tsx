import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import { fetchMyView, postGameAction, subscribeViewStream } from "./api";
import { TeamieHostBridge, type BridgeState } from "./bridge";
import {
  BlackVoteSealIcon,
  ChevronIcon,
  CrownLeaderIcon,
  ExcaliburSealEmblem,
  ExpeditionShieldIcon,
  EyeVeilIcon,
  HolyGrailIcon,
  RoleEmblem,
  RoundTableRunesSvg,
  SabotageDaggerIcon,
  ScrollLedgerIcon,
  ThemeSunMoonIcon,
  WhiteVoteSealIcon,
} from "./icons";
import type { PlayerView, ProposalRecord, Role, RoleOptions, SeatView } from "./types";

interface PresetCardMeta {
  value: string;
  name: string;
  meta: string;
}

const MAIN_PRESET_CARDS: PresetCardMeta[] = [
  { value: "5p", name: "5人标准局", meta: "3蓝2红 · 梅派忠 vs 莫刺" },
  { value: "6p", name: "6人标准局", meta: "4蓝2红 · 梅派2忠 vs 莫刺" },
  { value: "7p_oberon", name: "7人奥伯伦", meta: "4蓝3红 · Q4双败护盾" },
  { value: "7p_mordred", name: "7人莫德雷德", meta: "4蓝3红 · 隐匿黑老大" },
];

const EXTRA_PRESET_CARDS: PresetCardMeta[] = [
  { value: "8p", name: "8人圣战局", meta: "5蓝3红 · 莫刺黑" },
  { value: "9p", name: "9人王城局", meta: "6蓝3红 · 莫刺黑" },
  { value: "10p", name: "10人全神局", meta: "6蓝4红 · 莫刺黑奥" },
];

const ROLE_SWITCH_LABELS: { key: keyof RoleOptions; label: string; side: "good" | "evil" }[] = [
  { key: "merlin", label: "梅林", side: "good" },
  { key: "percival", label: "派西维尔", side: "good" },
  { key: "assassin", label: "刺客", side: "evil" },
  { key: "morgana", label: "莫甘娜", side: "evil" },
  { key: "mordred", label: "莫德雷德", side: "evil" },
  { key: "oberon", label: "奥伯伦", side: "evil" },
];

const ROLE_TITLES: Record<Role, string> = {
  MERLIN: "梅林",
  PERCIVAL: "派西维尔",
  SERVANT: "亚瑟忠臣",
  ASSASSIN: "暗影刺客",
  MORGANA: "莫甘娜",
  MORDRED: "莫德雷德",
  OBERON: "奥伯伦",
  MINION: "红方爪牙",
};

const ROLE_EPITHETS: Record<Role, string> = {
  MERLIN: "星辰先知 · 洞悉红方暗影，切忌暴露真身",
  PERCIVAL: "圣杯护卫 · 辨识真假梅林，替真先知挡刀",
  SERVANT: "忠诚骑士 · 无初始私密视野，凭票型护车",
  ASSASSIN: "终局杀手 · 潜伏搅局，蓝方三胜后拍刀梅林",
  MORGANA: "幻象魔女 · 伪装梅林迷惑派西维尔视线",
  MORDRED: "黑甲叛王 · 隐匿于梅林盲区，深藏不露",
  OBERON: "迷雾孤狼 · 与红方同伴双向隔绝，独自搅局",
  MINION: "暗影爪牙 · 协同红方同伴骗取车位炸车",
};

const SEAT_SEAL_COLORS = [
  "#2f68c4",
  "#8e54d1",
  "#25856e",
  "#bd3f52",
  "#b57b28",
  "#3b7a9e",
  "#9c487c",
  "#5a6b8c",
  "#a65732",
  "#4c5bd4",
];

function formatClock(sec: number): string {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

function buildLineupChips(view: PlayerView): { label: string; side: "good" | "evil" }[] {
  const opts = view.roleOptions;
  const good: { label: string; side: "good" | "evil" }[] = [];
  if (opts.merlin) good.push({ label: "梅林", side: "good" });
  if (opts.percival) good.push({ label: "派西维尔", side: "good" });
  const servants = Math.max(0, view.goodCount - good.length);
  if (servants > 0) {
    good.push({ label: servants > 1 ? `忠臣×${servants}` : "忠臣", side: "good" });
  }

  const evil: { label: string; side: "good" | "evil" }[] = [];
  if (opts.morgana) evil.push({ label: "莫甘娜", side: "evil" });
  if (opts.assassin) evil.push({ label: "刺客", side: "evil" });
  if (opts.mordred) evil.push({ label: "莫德雷德", side: "evil" });
  if (opts.oberon) evil.push({ label: "奥伯伦", side: "evil" });
  const minions = Math.max(0, view.evilCount - evil.length);
  if (minions > 0) {
    evil.push({ label: minions > 1 ? `爪牙×${minions}` : "爪牙", side: "evil" });
  }

  return [...good.slice(0, view.goodCount), ...evil.slice(0, view.evilCount)];
}

/** Compute clockwise polar seat coordinates (%) from 12 o'clock around the Round Table */
function computeSeatPolar(index: number, total: number): { xPct: number; yPct: number } {
  const safeTotal = Math.max(1, total);
  const angle = -Math.PI / 2 + (2 * Math.PI * index) / safeTotal;
  const rx = safeTotal >= 9 ? 37.5 : 36.5;
  const ry = safeTotal >= 9 ? 39.5 : 38;
  return {
    xPct: 50 + rx * Math.cos(angle),
    yPct: 50 + ry * Math.sin(angle),
  };
}

function LedgerPanel({
  view,
  reversedVotes,
  ledgerTab,
  setLedgerTab,
}: {
  view: PlayerView;
  reversedVotes: ProposalRecord[];
  ledgerTab: "votes" | "quests" | "events";
  setLedgerTab: (t: "votes" | "quests" | "events") => void;
}) {
  const seatNumbers = useMemo(() => {
    if (view.seats.length > 0) {
      return view.seats.map((s) => s.seat);
    }
    if (reversedVotes.length > 0) {
      return reversedVotes[0].votes.map((v) => v.seat);
    }
    return [];
  }, [view.seats, reversedVotes]);

  return (
    <div className="av-ledger-panel">
      <div className="av-tabs" role="tablist" aria-label="对局推演记录">
        <button
          type="button"
          role="tab"
          aria-selected={ledgerTab === "votes"}
          className={`av-tab ${ledgerTab === "votes" ? "active" : ""}`}
          onClick={() => setLedgerTab("votes")}
        >
          公开票型矩阵 ({view.voteHistory.length})
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={ledgerTab === "quests"}
          className={`av-tab ${ledgerTab === "quests" ? "active" : ""}`}
          onClick={() => setLedgerTab("quests")}
        >
          圣杯匿名开票 ({view.completedQuests.length})
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={ledgerTab === "events"}
          className={`av-tab ${ledgerTab === "events" ? "active" : ""}`}
          onClick={() => setLedgerTab("events")}
        >
          圆桌编年战报 ({view.recentEvents.length})
        </button>
      </div>

      {ledgerTab === "votes" && (
        <div className="av-ledger-scroll">
          {reversedVotes.length === 0 ? (
            <div className="av-empty-ledger">
              暂无公开公投记录。每次全员盖下白黑火漆章后，完整车队与座位票型将永久载入此推演矩阵。
            </div>
          ) : (
            <>
              <div className="av-matrix-table-wrap">
                <table className="av-matrix-table">
                  <thead>
                    <tr>
                      <th>车次</th>
                      <th>队长/车队</th>
                      {seatNumbers.map((sn) => (
                        <th key={sn} className="center">
                          #{sn}
                        </th>
                      ))}
                      <th className="right">结果</th>
                    </tr>
                  </thead>
                  <tbody>
                    {reversedVotes.map((rec, i) => {
                      const voteMap = new Map(rec.votes.map((v) => [v.seat, v]));
                      return (
                        <tr key={`${rec.questNumber}-${rec.attemptNumber}-${i}`}>
                          <td className="av-td-round">
                            Q{rec.questNumber}-{rec.attemptNumber}
                          </td>
                          <td className="av-td-team">
                            <span className="av-leader-mini">👑#{rec.leaderSeat}</span>
                            <span className="av-team-mini">
                              [{rec.teamSeats.map((s) => `#${s}`).join(",")}]
                            </span>
                          </td>
                          {seatNumbers.map((sn) => {
                            const entry = voteMap.get(sn);
                            const onTeam = rec.teamSeats.includes(sn);
                            if (!entry) {
                              return (
                                <td key={sn} className="center">
                                  -
                                </td>
                              );
                            }
                            return (
                              <td key={sn} className="center">
                                <span
                                  className={[
                                    "av-matrix-cell",
                                    entry.approve ? "approve" : "reject",
                                    onTeam ? "in-team" : "",
                                  ]
                                    .filter(Boolean)
                                    .join(" ")}
                                  title={`#${sn} ${entry.displayName}${
                                    onTeam ? " (本车成员)" : ""
                                  }：${entry.approve ? "白章赞成" : "黑章反对"}`}
                                >
                                  {onTeam ? "🛡️" : ""}
                                  {entry.approve ? "赞" : "反"}
                                </span>
                              </td>
                            );
                          })}
                          <td className="right">
                            <span
                              className={`av-outcome-tag ${rec.approved ? "pass" : "fail"}`}
                            >
                              {rec.approved
                                ? `发车 ${rec.approveCount}:${rec.rejectCount}`
                                : `流票 ${rec.approveCount}:${rec.rejectCount}`}
                            </span>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>

              {/* Speech stance notes archive */}
              {reversedVotes.some(
                (r) => r.speechNotes && r.speechNotes.some((n) => n.summary),
              ) && (
                <div className="av-stance-archive">
                  {reversedVotes
                    .filter((r) => r.speechNotes && r.speechNotes.some((n) => n.summary))
                    .map((r, idx) => (
                      <div key={idx} className="av-stance-block">
                        <div className="av-stance-head">
                          Q{r.questNumber}·第{r.attemptNumber}车 发言态度备忘：
                        </div>
                        {r.speechNotes!
                          .filter((n) => n.summary)
                          .map((n) => (
                            <div key={n.seat} className="av-stance-line">
                              <b>#{n.seat}</b> {n.displayName}：{n.summary}
                            </div>
                          ))}
                      </div>
                    ))}
                </div>
              )}
            </>
          )}
        </div>
      )}

      {ledgerTab === "quests" && (
        <div className="av-ledger-scroll">
          {view.completedQuests.length === 0 ? (
            <div className="av-empty-ledger">
              暂无已结算任务。圣杯远征洗牌开票仅公开成功与破坏张数，绝对隐藏出牌人映射。
            </div>
          ) : (
            [...view.completedQuests].reverse().map((q) => (
              <div key={q.questNumber} className="av-quest-rec-card">
                <div className="av-vote-head">
                  <span>
                    <b>第 {q.questNumber} 轮圣杯远征</b> · 👑#{q.leaderSeat} 车队 [
                    {q.teamSeats.map((s) => `#${s}`).join(", ")}]
                  </span>
                  <span className={`av-outcome-tag ${q.succeeded ? "pass" : "fail"}`}>
                    {q.succeeded ? "🏆 圣杯成功" : "🗡️ 任务陷落"}
                  </span>
                </div>
                <div className="av-quest-cards-row">
                  <span className="av-badge good">圣杯成功卡 × {q.successCount}</span>
                  <span className={`av-badge ${q.failCount > 0 ? "evil" : ""}`}>
                    暗影破坏卡 × {q.failCount}
                  </span>
                  {q.twoFailsRequired && <span className="av-badge gold">双败保护轮</span>}
                </div>
              </div>
            ))
          )}
        </div>
      )}

      {ledgerTab === "events" && (
        <div className="av-ledger-scroll">
          {view.recentEvents.length === 0 ? (
            <div className="av-empty-ledger">暂无圆桌编年战报。</div>
          ) : (
            [...view.recentEvents].reverse().map((ev, idx) => (
              <div key={idx} className="av-event-item">
                {ev}
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
}

export function App() {
  const [bridgeState, setBridgeState] = useState<BridgeState>({
    ready: false,
    hostOrigin: null,
    context: null,
  });
  const [view, setView] = useState<PlayerView | null>(null);
  const [sseConnected, setSseConnected] = useState(false);
  const [busy, setBusy] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [hideIdentity, setHideIdentity] = useState(false);
  const [showRoleCustomize, setShowRoleCustomize] = useState(false);
  const [showMorePresets, setShowMorePresets] = useState(false);

  // Adaptive OS theme with optional one-tap light/dark switch (memory-scoped)
  const [isLightTheme, setIsLightTheme] = useState<boolean>(() => {
    if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
      return window.matchMedia("(prefers-color-scheme: light)").matches;
    }
    return false;
  });
  const [manualTheme, setManualTheme] = useState<"dark" | "light" | null>(null);

  // Interactive selections & mobile ledger dock
  const [selectedTeam, setSelectedTeam] = useState<number[]>([]);
  const [speechNote, setSpeechNote] = useState("");
  const [assassinTarget, setAssassinTarget] = useState<number>(0);
  const [ledgerOpen, setLedgerOpen] = useState(false);
  const [ledgerTab, setLedgerTab] = useState<"votes" | "quests" | "events">("votes");

  // Smooth local countdown clock & fluid bar
  const [nowTickMs, setNowTickMs] = useState(() => Date.now());
  const [deadlineSpanMs, setDeadlineSpanMs] = useState(90_000);

  const bridgeRef = useRef<TeamieHostBridge | null>(null);

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") return;
    const mq = window.matchMedia("(prefers-color-scheme: light)");
    const handler = (ev: MediaQueryListEvent) => setIsLightTheme(ev.matches);
    mq.addEventListener?.("change", handler);
    return () => mq.removeEventListener?.("change", handler);
  }, []);

  const effectiveLight = manualTheme ? manualTheme === "light" : isLightTheme;

  useEffect(() => {
    if (manualTheme) {
      document.documentElement.setAttribute("data-theme", manualTheme);
    } else {
      document.documentElement.removeAttribute("data-theme");
    }
  }, [manualTheme]);

  useEffect(() => {
    const bridge = new TeamieHostBridge({
      onUpdate: (st) => {
        setBridgeState({ ...st });
      },
    });
    bridgeRef.current = bridge;
    bridge.start();
    return () => {
      bridge.stop();
      bridgeRef.current = null;
    };
  }, []);

  const token = bridgeState.context?.token ?? "";

  const handleUnauthorized = useCallback(() => {
    bridgeRef.current?.requestTokenRefresh(true);
  }, []);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;

    void fetchMyView(token, handleUnauthorized)
      .then((v) => {
        if (!cancelled) {
          setView(v);
          setErrorMsg(null);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setErrorMsg(err instanceof Error ? err.message : "加载圆桌状态失败");
        }
      });

    const unsubscribe = subscribeViewStream({
      token,
      onView: (v) => {
        if (!cancelled) {
          setView(v);
        }
      },
      onStatus: (ok) => {
        if (!cancelled) {
          setSseConnected(ok);
        }
      },
      onUnauthorized: handleUnauthorized,
    });

    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, [token, handleUnauthorized]);

  // Reset local picks when quest, proposal attempt, or phase shifts
  useEffect(() => {
    setSelectedTeam([]);
    setAssassinTarget(0);
  }, [view?.gameId, view?.questNumber, view?.proposalAttempt, view?.phase]);

  // Local countdown computed purely from phaseDeadlineUnixMs (zero server tick traffic)
  useEffect(() => {
    const deadline = view?.phaseDeadlineUnixMs;
    if (!deadline) return;
    const now = Date.now();
    setNowTickMs(now);
    const fallbackMs = Math.max(15, view?.phaseTimeoutSec || 90) * 1000;
    setDeadlineSpanMs(Math.max(fallbackMs, deadline - now));
    const id = window.setInterval(() => setNowTickMs(Date.now()), 400);
    return () => window.clearInterval(id);
  }, [view?.phaseDeadlineUnixMs, view?.phaseTimeoutSec]);

  const remainSec = useMemo(() => {
    if (!view?.phaseDeadlineUnixMs) return null;
    return Math.max(0, Math.ceil((view.phaseDeadlineUnixMs - nowTickMs) / 1000));
  }, [view?.phaseDeadlineUnixMs, nowTickMs]);

  const countdownPct = useMemo(() => {
    if (!view?.phaseDeadlineUnixMs || deadlineSpanMs <= 0) return 0;
    const left = Math.max(0, view.phaseDeadlineUnixMs - nowTickMs);
    return Math.min(100, Math.max(0, (left / deadlineSpanMs) * 100));
  }, [view?.phaseDeadlineUnixMs, nowTickMs, deadlineSpanMs]);

  const runAction = useCallback(
    async (action: string, payload: Record<string, unknown> = {}) => {
      if (!token || busy) return;
      setBusy(true);
      setErrorMsg(null);
      try {
        const res = await postGameAction(token, action, payload, handleUnauthorized);
        setView(res.view);
      } catch (err: unknown) {
        setErrorMsg(err instanceof Error ? err.message : "执行操作失败");
      } finally {
        setBusy(false);
      }
    },
    [token, busy, handleUnauthorized],
  );

  const toggleTeamSeat = (seat: number, maxSize: number) => {
    setSelectedTeam((prev) => {
      if (prev.includes(seat)) {
        return prev.filter((s) => s !== seat);
      }
      if (prev.length >= maxSize) {
        return [...prev.slice(1), seat].sort((a, b) => a - b);
      }
      return [...prev, seat].sort((a, b) => a - b);
    });
  };

  const reversedVotes = useMemo(() => {
    if (!view?.voteHistory) return [];
    return [...view.voteHistory].reverse();
  }, [view?.voteHistory]);

  // Handshake Waiting Screen
  if (!bridgeState.ready || !bridgeState.context) {
    return (
      <main className="av-shell">
        <section className="av-wait-screen">
          <div className="av-crest-ring">
            <ExcaliburSealEmblem className="av-svg-md" style={{ width: 32, height: 32 }} />
          </div>
          <strong style={{ fontSize: "16px", color: "var(--gold-bright)" }}>
            阿瓦隆 · 卡美洛圆桌裁判台
          </strong>
          <span style={{ fontSize: "13px", color: "var(--text-secondary)", maxWidth: 340 }}>
            正在等待 Teamie 群聊宿主下发安全会话凭证（<code>teamie:mcp-app:init</code>）…
          </span>
          <small style={{ fontSize: "11.5px", color: "var(--text-faint)", maxWidth: 320 }}>
            支持动态面板宽度自适应展开，兼容 5–10 名人类与 Eino Agent 同桌圆桌推理。
          </small>
        </section>
      </main>
    );
  }

  if (!view) {
    return (
      <main className="av-shell">
        <section className="av-wait-screen">
          <div className="av-crest-ring">
            <HolyGrailIcon className="av-svg-md" style={{ width: 30, height: 30 }} />
          </div>
          <strong style={{ color: "var(--gold-bright)" }}>正在铺设圣杯圆桌状态机…</strong>
          {errorMsg && <div className="av-toast-err">{errorMsg}</div>}
        </section>
      </main>
    );
  }

  const { you } = view;
  const teamSizeNeeded = view.currentTeamSize ?? 2;
  const visibleEvilSeats = you.privateVision?.visibleEvilSeats ?? [];
  const merlinCandidateSeats = you.privateVision?.merlinCandidateSeats ?? [];
  const knownEvilTeammates = you.privateVision?.evilTeammateSeats ?? [];

  const seatNameByNum = (seatNum: number): string => {
    const found = view.seats.find((s) => s.seat === seatNum);
    return found ? `#${seatNum} ${found.displayName}` : `#${seatNum}`;
  };

  // Interactive direct-on-seat targeting modes
  const canPickTeam = view.phase === "TEAM_PROPOSAL" && Boolean(you.isLeader);
  const canPickAssassin = view.phase === "ASSASSINATION" && you.role === "ASSASSIN";

  const isSeatClickable = (s: SeatView): boolean => {
    if (canPickTeam) return true;
    if (canPickAssassin) {
      return s.seat !== you.seat && !knownEvilTeammates.includes(s.seat);
    }
    return false;
  };

  const handleSeatClick = (s: SeatView) => {
    if (canPickTeam) {
      toggleTeamSeat(s.seat, teamSizeNeeded);
    } else if (canPickAssassin && isSeatClickable(s)) {
      setAssassinTarget((prev) => (prev === s.seat ? 0 : s.seat));
    }
  };

  const shellMoodClass =
    view.phase === "GAME_OVER"
      ? view.winner === "GOOD"
        ? "is-win-good"
        : "is-win-evil"
      : view.phase === "ASSASSINATION" || view.isHammerProposal
        ? "is-danger"
        : "";

  const crestMoodClass =
    view.phase === "ASSASSINATION" || view.isHammerProposal
      ? "danger"
      : view.phase === "QUEST_EXECUTION" || (view.phase === "GAME_OVER" && view.winner === "GOOD")
        ? "good"
        : "";

  const totalRingSlots =
    view.phase === "LOBBY"
      ? Math.max(5, view.targetPlayerCount, view.seats.length)
      : Math.max(1, view.seats.length);

  const emptySeatCount =
    view.phase === "LOBBY" ? Math.max(0, totalRingSlots - view.seats.length) : 0;

  const lineupChips = view.phase === "LOBBY" ? buildLineupChips(view) : [];
  const latestEvent =
    view.recentEvents.length > 0
      ? view.recentEvents[view.recentEvents.length - 1]
      : "圣杯圆桌就绪，等待骑士入座缔结契约";

  const isExtraPresetActive = EXTRA_PRESET_CARDS.some((p) => p.value === view.preset);

  // Active Expedition Seats for radial table spoke beams
  const activeExpeditionSeats = canPickTeam
    ? selectedTeam
    : (view.proposedTeamSeats ?? []);

  const votedCount = view.seats.filter((s) => s.hasVotedTeam).length;

  return (
    <main className={`av-shell ${shellMoodClass}`}>
      {/* 1. Full-Width Sticky Ornate Header */}
      <header className="av-topbar">
        <div className="av-topbar-inner">
          <div className="av-brand-cluster">
            <span
              className={`av-live-dot ${sseConnected ? "online" : "offline"}`}
              title={sseConnected ? "实时事件流已连接" : "实时流重连中"}
            />
            <span className="av-brand-wordmark">
              <ExcaliburSealEmblem className="av-svg-sm" />
              <span>阿瓦隆圆桌</span>
            </span>
            <span className={`av-phase-crest ${crestMoodClass}`}>
              {view.phase === "ASSASSINATION" ? (
                <SabotageDaggerIcon className="av-svg-sm" />
              ) : view.phase === "QUEST_EXECUTION" ? (
                <HolyGrailIcon className="av-svg-sm" />
              ) : (
                <CrownLeaderIcon className="av-svg-sm" />
              )}
              <span>
                {view.questNumber && view.phase !== "LOBBY" && view.phase !== "GAME_OVER"
                  ? `Q${view.questNumber} · `
                  : ""}
                {view.phaseLabel}
              </span>
            </span>
          </div>

          <div className="av-topbar-right">
            <span className="av-score-banner">
              <span style={{ color: "var(--good)" }}>🏆 {view.successQuestCount}</span>
              <span style={{ color: "var(--text-faint)" }}>:</span>
              <span style={{ color: "var(--evil)" }}>🗡️ {view.failQuestCount}</span>
            </span>

            <span className="av-ratio-pill" title="蓝方好人 vs 红方坏人配比">
              <span style={{ color: "var(--good)" }}>{view.goodCount}蓝</span>
              <span style={{ color: "var(--text-faint)" }}>:</span>
              <span style={{ color: "var(--evil)" }}>{view.evilCount}红</span>
            </span>

            {remainSec !== null && view.phase !== "LOBBY" && view.phase !== "GAME_OVER" && (
              <span className={`av-timer-pill ${remainSec <= 15 ? "urgent" : ""}`}>
                {formatClock(remainSec)}
              </span>
            )}

            <button
              type="button"
              className="av-icon-btn"
              title={effectiveLight ? "切换为暗夜黑曜圆桌主题" : "切换为羊皮圣卷浅色主题"}
              aria-label="切换深浅色主题"
              onClick={() => setManualTheme(effectiveLight ? "dark" : "light")}
            >
              <ThemeSunMoonIcon isLight={effectiveLight} className="av-svg-sm" />
            </button>
          </div>
        </div>

        {remainSec !== null && view.phase !== "LOBBY" && view.phase !== "GAME_OVER" && (
          <div className="av-countdown-track" aria-hidden="true">
            <div
              className={`av-countdown-fill ${remainSec <= 15 ? "urgent" : ""}`}
              style={{ width: `${countdownPct}%` }}
            />
          </div>
        )}
      </header>

      {/* 2. Dynamic-Width Responsive Camelot War Room */}
      <div className="av-workspace">
        {/* LEFT / MAIN ARENA PANE: 5-Quest Holy Grail Board + Circular Round Table */}
        <div className="av-arena-pane">
          {errorMsg && (
            <div className="av-toast-err" role="alert">
              <span>{errorMsg}</span>
              <button
                type="button"
                className="av-btn-mini"
                onClick={() => setErrorMsg(null)}
                aria-label="关闭错误提示"
              >
                ✕
              </button>
            </div>
          )}

          {/* Ceremonial 5-Quest Grail Board & 5-Reject Wax Track */}
          <section className="av-board-mat" aria-label="圣杯任务与流票轨">
            <div className="av-board-head">
              <span className="av-board-title">
                <HolyGrailIcon className="av-svg-sm" />
                <span>五轮圣杯远征进度轨</span>
              </span>
              <div className="av-score-pills">
                <span className="av-score-good">圣杯成功 {view.successQuestCount}/3</span>
                <span className="av-score-evil">任务陷落 {view.failQuestCount}/3</span>
              </div>
            </div>

            <div className="av-quest-rail">
              {view.questTrack.map((slot) => (
                <div
                  key={slot.questNumber}
                  className={`av-quest-slot status-${slot.status.toLowerCase()}`}
                  title={
                    slot.twoFailsRequired
                      ? `第 ${slot.questNumber} 轮：需 ${slot.teamSize} 人上车（保护轮：至少 2 张破坏卡才算失败）`
                      : `第 ${slot.questNumber} 轮：需 ${slot.teamSize} 人上车`
                  }
                >
                  <div className="av-quest-top">
                    <span>第{slot.questNumber}轮</span>
                    {slot.twoFailsRequired && <span className="av-protect-tag">2败护盾</span>}
                  </div>

                  <div className="av-quest-medallion">
                    {slot.status === "SUCCEEDED" ? (
                      <HolyGrailIcon className="av-svg-md" />
                    ) : slot.status === "FAILED" ? (
                      <SabotageDaggerIcon className="av-svg-md" />
                    ) : (
                      slot.teamSize
                    )}
                  </div>

                  <div className="av-quest-foot">
                    {slot.successCount !== undefined && slot.failCount !== undefined
                      ? `${slot.successCount}成 · ${slot.failCount}败`
                      : `${slot.teamSize}人远征`}
                  </div>
                </div>
              ))}
            </div>

            <div className="av-reject-wrap">
              <div className="av-reject-head">
                <span>流票计数轨（同轮连续 5 次否决车队则红方直接获胜）</span>
                <span
                  style={{
                    fontWeight: 700,
                    color: view.isHammerProposal ? "var(--evil)" : "var(--text-secondary)",
                  }}
                >
                  {view.proposalAttempt ? `第 ${view.proposalAttempt} 任队长 · ` : ""}
                  流票 {view.rejectedCount} / 5
                </span>
              </div>

              <div className="av-reject-pips">
                {[1, 2, 3, 4, 5].map((idx) => (
                  <div
                    key={idx}
                    className={[
                      "av-reject-pip",
                      idx <= view.rejectedCount ? "filled" : "",
                      idx === 5 ? "hammer" : "",
                      idx === 5 && view.isHammerProposal ? "armed" : "",
                    ]
                      .filter(Boolean)
                      .join(" ")}
                  >
                    {idx === 5 ? "5 · 强制尾车 (Hammer)" : `流票 ${idx}`}
                  </div>
                ))}
              </div>

              {view.isHammerProposal && (
                <div className="av-hammer-alert">
                  ⚡ 尾车警报（Hammer）：当前已是本轮第 5 任尾置位队长！若本车再被公投否决，红方将直接获胜！
                </div>
              )}
            </div>
          </section>

          {/* Circular Knights of the Round Table Arena (adapts to 2-col grid below 560px) */}
          <section className="av-table-section" aria-label="卡美洛圆桌骑士席">
            <div className="av-section-head">
              <span className="av-section-title">
                卡美洛圆桌骑士席（{view.seats.length}/{totalRingSlots} · 顺时针序）
              </span>

              {canPickTeam ? (
                <span className="av-section-hint">
                  请直接点击圆桌骑士点选车队（{selectedTeam.length}/{teamSizeNeeded}）
                </span>
              ) : canPickAssassin ? (
                <span className="av-section-hint danger">请直接点击可疑蓝方座位锁定刺杀</span>
              ) : view.leaderSeat ? (
                <span className="av-section-hint">👑 本任队长：#{view.leaderSeat}</span>
              ) : null}
            </div>

            <div
              className={`av-round-table-ring count-${totalRingSlots} ${
                totalRingSlots >= 9 ? "dense-ring" : ""
              }`}
            >
              {/* SVG Radial Beams connecting Center Round Table to Leader / Expedition Team / Assassin Aim */}
              <svg
                className="av-table-spokes"
                viewBox="0 0 100 100"
                preserveAspectRatio="none"
                aria-hidden="true"
              >
                {view.seats.map((s, idx) => {
                  const { xPct, yPct } = computeSeatPolar(idx, totalRingSlots);
                  const inExp = activeExpeditionSeats.includes(s.seat);
                  const isAssTarget = canPickAssassin && assassinTarget === s.seat;
                  if (!inExp && !s.isLeader && !isAssTarget) return null;
                  return (
                    <line
                      key={s.seat}
                      x1="50"
                      y1="50"
                      x2={xPct}
                      y2={yPct}
                      className={
                        isAssTarget
                          ? "spoke-assassin"
                          : inExp
                            ? "spoke-team"
                            : "spoke-leader"
                      }
                    />
                  );
                })}
              </svg>

              {/* Carved Centerpiece of the Arthurian Round Table */}
              <div className="av-table-center" aria-hidden="true">
                <RoundTableRunesSvg className="av-table-runes" />
                <div className="av-center-readout">
                  <span className="av-center-kicker">
                    {view.phase === "LOBBY"
                      ? "CAMELOT HALL"
                      : view.phase === "GAME_OVER"
                        ? "FINAL VERDICT"
                        : `QUEST ${view.questNumber ?? 1}`}
                  </span>
                  <strong className="av-center-main">
                    {view.phase === "LOBBY"
                      ? `${view.seats.length}/${totalRingSlots} 入座`
                      : view.phase === "TEAM_PROPOSAL"
                        ? `👑 #${view.leaderSeat} 组队`
                        : view.phase === "PROPOSAL_SPEECH"
                          ? `🎙️ #${view.currentSpeakerSeat} 陈词`
                          : view.phase === "TEAM_VOTE"
                            ? `⚖️ 已投 ${votedCount}/${view.seats.length}`
                            : view.phase === "QUEST_EXECUTION"
                              ? `🏆 出牌 ${view.questSubmittedCount ?? 0}/${
                                  view.questRequiredCards ?? teamSizeNeeded
                                }`
                              : view.phase === "ASSASSINATION"
                                ? "🗡️ 刺客亮刀"
                                : view.winner === "GOOD"
                                  ? "🏆 蓝方圣胜"
                                  : "🔥 红方颠覆"}
                  </strong>
                  {activeExpeditionSeats.length > 0 && view.phase !== "GAME_OVER" && (
                    <span className="av-center-sub">
                      远征队：{activeExpeditionSeats.map((n) => `#${n}`).join(" · ")}
                    </span>
                  )}
                </div>
              </div>

              {/* Clockwise Radial Knight Seat Cards */}
              {view.seats.map((s, idx) => {
                const { xPct, yPct } = computeSeatPolar(idx, totalRingSlots);
                const polarStyle = {
                  "--seat-x": `${xPct}%`,
                  "--seat-y": `${yPct}%`,
                } as CSSProperties;

                const clickable = isSeatClickable(s);
                const pickedForTeam = canPickTeam && selectedTeam.includes(s.seat);
                const pickedForAssassin = canPickAssassin && assassinTarget === s.seat;
                const sealColor = SEAT_SEAL_COLORS[(s.seat - 1) % SEAT_SEAL_COLORS.length];

                const showVisionOnSeat = !hideIdentity && view.phase !== "GAME_OVER";
                const isSeenEvil = showVisionOnSeat && visibleEvilSeats.includes(s.seat);
                const isMerlinCand = showVisionOnSeat && merlinCandidateSeats.includes(s.seat);
                const isEvilAlly =
                  showVisionOnSeat && knownEvilTeammates.includes(s.seat) && s.seat !== you.seat;

                return (
                  <button
                    key={s.seat}
                    type="button"
                    style={polarStyle}
                    disabled={!clickable}
                    onClick={() => handleSeatClick(s)}
                    className={[
                      "av-seat-card",
                      clickable ? "can-pick" : "",
                      s.seat === you.seat ? "is-me" : "",
                      s.onProposedTeam && !pickedForTeam ? "on-team" : "",
                      pickedForTeam ? "is-picked-team" : "",
                      pickedForAssassin ? "is-picked-assassin" : "",
                      s.isCurrentSpeaker ? "is-speaking" : "",
                    ]
                      .filter(Boolean)
                      .join(" ")}
                  >
                    <div className="av-seat-top">
                      <div className="av-seat-id">
                        <span className="av-seat-seal" style={{ background: sealColor }}>
                          {s.seat}
                        </span>
                        <span className="av-seat-name" title={s.displayName}>
                          {s.displayName}
                        </span>
                      </div>

                      <div className="av-seat-icons">
                        {s.isLeader && (
                          <span className="av-crown-token" title="本任队长">
                            <CrownLeaderIcon className="av-svg-sm" />
                          </span>
                        )}
                        {(s.onProposedTeam || pickedForTeam) && (
                          <span className="av-shield-token" title="远征车队成员">
                            <ExpeditionShieldIcon className="av-svg-sm" />
                          </span>
                        )}
                        {s.isCurrentSpeaker && (
                          <span className="av-eq-bars" title="正在发言">
                            <span className="av-eq-bar b1" />
                            <span className="av-eq-bar b2" />
                            <span className="av-eq-bar b3" />
                          </span>
                        )}
                      </div>
                    </div>

                    <div className="av-seat-bottom">
                      <div
                        style={{
                          display: "inline-flex",
                          gap: "3px",
                          flexWrap: "wrap",
                          alignItems: "center",
                        }}
                      >
                        {s.seat === you.seat && <span className="av-badge gold">我</span>}
                        {s.isHost && view.phase === "LOBBY" && (
                          <span className="av-badge gold">房主</span>
                        )}
                        {s.isLeader && <span className="av-badge gold">队长</span>}
                        {s.onProposedTeam && !canPickTeam && (
                          <span className="av-badge good">车队</span>
                        )}
                        {view.phase === "PROPOSAL_SPEECH" &&
                          s.hasSpokenThisTurn &&
                          !s.isCurrentSpeaker && <span className="av-badge emerald">✓已言</span>}
                        {view.phase === "TEAM_VOTE" && (
                          <span className={`av-badge ${s.hasVotedTeam ? "emerald" : ""}`}>
                            {s.hasVotedTeam ? "✓已落章" : "待投票"}
                          </span>
                        )}
                        {s.lastPublicVote !== undefined && view.phase !== "TEAM_VOTE" && (
                          <span className={`av-badge ${s.lastPublicVote ? "good" : "evil"}`}>
                            {s.lastPublicVote ? "⚪上轮赞" : "⚫上轮反"}
                          </span>
                        )}
                        {view.phase === "LOBBY" && s.seat !== you.seat && !s.isHost && (
                          <span className="av-seat-idle">候命</span>
                        )}
                        {view.phase !== "LOBBY" &&
                          s.seat !== you.seat &&
                          !s.isLeader &&
                          !s.onProposedTeam &&
                          view.phase !== "TEAM_VOTE" &&
                          s.lastPublicVote === undefined &&
                          !(view.phase === "PROPOSAL_SPEECH" && s.hasSpokenThisTurn) && (
                            <span className="av-seat-idle">
                              {s.isCurrentSpeaker ? "陈词中" : "在席"}
                            </span>
                          )}
                      </div>

                      {pickedForTeam ? (
                        <span className="av-badge aim-team">✓上车</span>
                      ) : pickedForAssassin ? (
                        <span className="av-badge aim-danger">🎯刺杀</span>
                      ) : s.revealedRole ? (
                        <span
                          className={`av-badge ${
                            s.revealedAlignment === "GOOD" ? "good" : "evil"
                          }`}
                        >
                          {ROLE_TITLES[s.revealedRole] || s.revealedRoleName}
                        </span>
                      ) : isSeenEvil ? (
                        <span className="av-badge evil">红方暗影</span>
                      ) : isMerlinCand ? (
                        <span className="av-badge mystic">梅林候选</span>
                      ) : isEvilAlly ? (
                        <span className="av-badge evil">红方同盟</span>
                      ) : null}
                    </div>
                  </button>
                );
              })}

              {/* Empty Knight Placeholders around the Round Table in Lobby */}
              {Array.from({ length: emptySeatCount }).map((_, idx) => {
                const ringIdx = view.seats.length + idx;
                const slotNum = ringIdx + 1;
                const { xPct, yPct } = computeSeatPolar(ringIdx, totalRingSlots);
                const polarStyle = {
                  "--seat-x": `${xPct}%`,
                  "--seat-y": `${yPct}%`,
                } as CSSProperties;

                return (
                  <div
                    key={`empty-${slotNum}`}
                    style={polarStyle}
                    className="av-seat-card is-empty-slot"
                  >
                    <div className="av-seat-top">
                      <div className="av-seat-id">
                        <span
                          className="av-seat-seal"
                          style={{
                            background: "var(--gold-soft)",
                            color: "var(--text-muted)",
                            borderColor: "var(--border)",
                          }}
                        >
                          {slotNum}
                        </span>
                        <span style={{ fontSize: "11.5px", color: "var(--text-faint)" }}>
                          虚位以待
                        </span>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          </section>
        </div>

        {/* RIGHT TACTICAL & DEDUCTION PANE: Tarot Identity + Command Altar + Always-Open Vote Matrix */}
        <aside className="av-side-pane" aria-label="身份视野、圣坛指令与票型推演">
          {/* A. Tarot Secret Identity & Fog-of-War Card */}
          {you.role ? (
            <button
              type="button"
              className={[
                "av-tarot-card",
                hideIdentity
                  ? "is-concealed"
                  : `${you.alignment === "GOOD" ? "align-good" : "align-evil"} role-${you.role}`,
              ].join(" ")}
              onClick={() => setHideIdentity((v) => !v)}
              title="点击暗置/翻开底牌与暗牌视野（防窥屏）"
            >
              <div className="av-tarot-main">
                <div className="av-tarot-left">
                  <div className="av-medallion">
                    <RoleEmblem role={hideIdentity ? undefined : you.role} className="av-svg-md" />
                  </div>
                  <div className="av-tarot-copy">
                    <div className="av-tarot-title">
                      <span>{you.seat ? `#${you.seat}` : ""}</span>
                      <span>
                        {hideIdentity
                          ? "圣剑密卷已暗置防窥"
                          : `${you.alignment === "GOOD" ? "亚瑟蓝方" : "莫德雷德红方"} · ${
                              ROLE_TITLES[you.role] || you.roleName
                            }`}
                      </span>
                    </div>
                    <div className="av-tarot-sub">
                      {hideIdentity
                        ? "点击翻开查看你的身份纹章与非对称视野线索"
                        : ROLE_EPITHETS[you.role]}
                    </div>
                  </div>
                </div>

                <span className="av-veil-pill">
                  <EyeVeilIcon hidden={hideIdentity} className="av-svg-sm" />
                  <span>{hideIdentity ? "翻开" : "暗置"}</span>
                </span>
              </div>

              {!hideIdentity &&
                (visibleEvilSeats.length > 0 ||
                  merlinCandidateSeats.length > 0 ||
                  knownEvilTeammates.length > 0) && (
                  <div className="av-vision-drawer">
                    {visibleEvilSeats.length > 0 && (
                      <>
                        <span className="av-vision-tag-label">洞悉红方(不含莫德雷德)：</span>
                        {visibleEvilSeats.map((s) => (
                          <span key={s} className="av-clue-chip evil">
                            {seatNameByNum(s)}
                          </span>
                        ))}
                      </>
                    )}

                    {merlinCandidateSeats.length > 0 && (
                      <>
                        <span className="av-vision-tag-label">真假梅林候选(已乱序)：</span>
                        {merlinCandidateSeats.map((s) => (
                          <span key={s} className="av-clue-chip mystic">
                            {seatNameByNum(s)}
                          </span>
                        ))}
                      </>
                    )}

                    {knownEvilTeammates.length > 0 && (
                      <>
                        <span className="av-vision-tag-label">红方同盟(不含奥伯伦)：</span>
                        {knownEvilTeammates.map((s) => (
                          <span key={s} className="av-clue-chip evil">
                            {seatNameByNum(s)}
                          </span>
                        ))}
                      </>
                    )}
                  </div>
                )}
            </button>
          ) : (
            <div className="av-lobby-strip">
              <div className="av-tarot-left">
                <div className="av-medallion">
                  <ExcaliburSealEmblem className="av-svg-md" />
                </div>
                <div className="av-tarot-copy">
                  <div className="av-tarot-title">
                    {you.seated ? `#${you.seat} · ${you.displayName}` : you.displayName}
                  </div>
                  <div className="av-tarot-sub">
                    {you.seated
                      ? you.isHost
                        ? "圆桌房主 · 可配置契约板子并拔剑发牌"
                        : "已入座圆桌 · 等待房主拔剑发牌"
                      : "旁观席 · 点击下方入座参与圆桌推理"}
                  </div>
                </div>
              </div>
              <span className={`av-badge ${you.seated ? "gold" : ""}`}>
                {you.seated ? `#${you.seat} 骑士` : "旁观"}
              </span>
            </div>
          )}

          {/* B. Contextual Camelot Command Altar */}
          <section
            className={[
              "av-altar",
              view.phase === "ASSASSINATION" ? "danger-altar" : "",
              view.phase === "GAME_OVER"
                ? view.winner === "GOOD"
                  ? "victory-good"
                  : "victory-evil"
                : "",
            ]
              .filter(Boolean)
              .join(" ")}
            aria-label="阶段圣坛操作台"
          >
            {/* Public Chat Discipline Strip */}
            <div
              className={[
                "av-policy-strip",
                you.isCurrentSpeaker
                  ? "my-mic"
                  : !view.publicChatPolicy.allowed
                    ? "locked"
                    : "",
              ]
                .filter(Boolean)
                .join(" ")}
            >
              <span className="av-policy-tag">
                {you.isCurrentSpeaker
                  ? "🎙️ 你持麦"
                  : view.publicChatPolicy.allowed
                    ? "💬 群聊开放"
                    : "🤫 禁言落子"}
              </span>
              <span>{view.publicChatPolicy.guidance}</span>
            </div>

            {/* 1. LOBBY ALTAR */}
            {view.phase === "LOBBY" && (
              <>
                <div className="av-lineup-preview">
                  <span style={{ color: "var(--text-muted)", fontWeight: 700 }}>本局阵容：</span>
                  {lineupChips.map((chip, idx) => (
                    <span key={`${chip.label}-${idx}`} className={`av-badge ${chip.side}`}>
                      {chip.label}
                    </span>
                  ))}
                </div>

                <div className="av-btn-row">
                  {!you.seated ? (
                    <button
                      type="button"
                      className="av-btn av-btn-gold av-w-full"
                      disabled={busy || view.seats.length >= 10}
                      onClick={() => void runAction("join_lobby", {})}
                    >
                      <ExcaliburSealEmblem className="av-svg-sm" />
                      <span>
                        入座圆桌契约（{view.seats.length}/{view.targetPlayerCount}）
                      </span>
                    </button>
                  ) : (
                    <>
                      <button
                        type="button"
                        className="av-btn"
                        disabled={busy}
                        onClick={() => void runAction("leave_lobby", {})}
                      >
                        离席 (#{you.seat})
                      </button>
                      {view.seats.length >= 5 &&
                      (you.isHost || view.seats.length === view.targetPlayerCount) ? (
                        <button
                          type="button"
                          className="av-btn av-btn-gold av-flex-1"
                          disabled={busy}
                          onClick={() => void runAction("start_game", {})}
                        >
                          <span>✦ 拔出石中剑 · 发牌开局 ({view.seats.length}人)</span>
                        </button>
                      ) : (
                        <div className="av-wait-note av-flex-1" style={{ textAlign: "center" }}>
                          等待凑齐至少 5 名骑士（当前 {view.seats.length} 人）
                        </div>
                      )}
                    </>
                  )}
                </div>

                {(you.isHost || view.seats.length === 0) && (
                  <>
                    <div className="av-altar-head" style={{ paddingTop: "2px" }}>
                      <span className="av-altar-title">圆桌契约板子预设</span>
                      <div style={{ display: "inline-flex", gap: "5px" }}>
                        <button
                          type="button"
                          className="av-btn-mini"
                          onClick={() => setShowMorePresets((v) => !v)}
                        >
                          {showMorePresets || isExtraPresetActive ? "收起8–10人" : "8–10人板"}
                        </button>
                        <button
                          type="button"
                          className="av-btn-mini"
                          onClick={() => setShowRoleCustomize((v) => !v)}
                        >
                          {showRoleCustomize ? "收起微调" : "神职/超时"}
                        </button>
                      </div>
                    </div>

                    <div className="av-preset-grid">
                      {[
                        ...MAIN_PRESET_CARDS,
                        ...(showMorePresets || isExtraPresetActive ? EXTRA_PRESET_CARDS : []),
                      ].map((p) => (
                        <button
                          key={p.value}
                          type="button"
                          disabled={busy}
                          className={`av-preset-btn ${view.preset === p.value ? "active" : ""}`}
                          onClick={() => void runAction("configure_lobby", { preset: p.value })}
                        >
                          <span className="av-preset-name">{p.name}</span>
                          <span className="av-preset-meta">{p.meta}</span>
                        </button>
                      ))}
                    </div>

                    {showRoleCustomize && (
                      <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
                        <div className="av-role-toggle-grid">
                          {ROLE_SWITCH_LABELS.map((item) => {
                            const active = view.roleOptions[item.key];
                            return (
                              <button
                                key={item.key}
                                type="button"
                                disabled={busy}
                                className={`av-role-chip-btn ${active ? `on-${item.side}` : ""}`}
                                onClick={() => {
                                  const nextOpts: RoleOptions = {
                                    ...view.roleOptions,
                                    [item.key]: !active,
                                  };
                                  void runAction("configure_lobby", {
                                    preset: "custom",
                                    roleOptions: nextOpts,
                                  });
                                }}
                              >
                                <span>{active ? "✓" : "+"}</span>
                                <span>{item.label}</span>
                              </button>
                            );
                          })}
                        </div>

                        <div className="av-btn-row" style={{ justifyContent: "space-between" }}>
                          <span className="av-altar-sub">单阶段防卡房超时：</span>
                          <div style={{ display: "inline-flex", gap: "4px" }}>
                            {[60, 90, 120, 180].map((sec) => (
                              <button
                                key={sec}
                                type="button"
                                disabled={busy}
                                className="av-btn-mini"
                                style={
                                  view.phaseTimeoutSec === sec
                                    ? {
                                        borderColor: "var(--gold)",
                                        color: "var(--gold-bright)",
                                        fontWeight: 800,
                                      }
                                    : undefined
                                }
                                onClick={() =>
                                  void runAction("configure_lobby", { phaseTimeoutSec: sec })
                                }
                              >
                                {sec}s
                              </button>
                            ))}
                          </div>
                        </div>
                      </div>
                    )}
                  </>
                )}
              </>
            )}

            {/* 2. TEAM_PROPOSAL ALTAR */}
            {view.phase === "TEAM_PROPOSAL" && (
              <>
                <div className="av-altar-head">
                  <span className="av-altar-title">
                    <CrownLeaderIcon className="av-svg-sm" />
                    <span>
                      {you.isLeader
                        ? `你是本任队长 · 请在圆桌点选 ${teamSizeNeeded} 名骑士`
                        : `队长 #${view.leaderSeat} 正在点选远征车队`}
                    </span>
                  </span>
                  {you.isLeader && selectedTeam.length > 0 && (
                    <button
                      type="button"
                      className="av-btn-mini"
                      onClick={() => setSelectedTeam([])}
                    >
                      清空重选
                    </button>
                  )}
                </div>

                {you.isLeader ? (
                  <>
                    <div className="av-draft-slots">
                      {Array.from({ length: teamSizeNeeded }).map((_, idx) => {
                        const pickedSeat = selectedTeam[idx];
                        return pickedSeat ? (
                          <span key={idx} className="av-draft-slot">
                            {seatNameByNum(pickedSeat)}
                          </span>
                        ) : (
                          <span key={idx} className="av-draft-slot empty">
                            待点选骑士 #{idx + 1}
                          </span>
                        );
                      })}
                    </div>

                    <button
                      type="button"
                      className="av-btn av-btn-sapphire av-w-full"
                      disabled={busy || selectedTeam.length !== teamSizeNeeded}
                      onClick={() => void runAction("propose_team", { teamSeats: selectedTeam })}
                    >
                      <ExpeditionShieldIcon className="av-svg-sm" />
                      <span>
                        确认提名远征车队（{selectedTeam.length}/{teamSizeNeeded}）
                      </span>
                    </button>
                  </>
                ) : (
                  <div className="av-wait-note">{you.actionHint}</div>
                )}
              </>
            )}

            {/* 3. PROPOSAL_SPEECH ALTAR */}
            {view.phase === "PROPOSAL_SPEECH" && (
              <>
                <div className="av-altar-head">
                  <span className="av-altar-title">顺时针圆桌陈词麦序</span>
                  <span className="av-altar-sub">
                    提名车队：{(view.proposedTeamSeats ?? []).map((s) => `#${s}`).join("、")}
                  </span>
                </div>

                {view.speechOrder && (
                  <div className="av-mic-relay">
                    {view.speechOrder.map((s, idx) => {
                      const isCur = s === view.currentSpeakerSeat;
                      const done = (view.currentSpeechNotes ?? []).some((n) => n.seat === s);
                      return (
                        <span
                          key={s}
                          style={{ display: "inline-flex", alignItems: "center", gap: "3px" }}
                        >
                          <span
                            className={`av-mic-step ${isCur ? "active" : done ? "done" : ""}`}
                          >
                            {isCur ? "🎙️" : done ? "✓" : ""}#{s}
                            {idx === view.speechOrder!.length - 1 ? "·总结" : ""}
                          </span>
                          {idx < view.speechOrder!.length - 1 && (
                            <span className="av-mic-arrow">→</span>
                          )}
                        </span>
                      );
                    })}
                  </div>
                )}

                {you.isCurrentSpeaker ? (
                  <>
                    <input
                      type="text"
                      className="av-input"
                      maxLength={80}
                      placeholder="可选：填写一句话站边/态度备忘（如：保1踩4，赞成此车）"
                      value={speechNote}
                      onChange={(e) => setSpeechNote(e.target.value)}
                    />
                    <button
                      type="button"
                      className="av-btn av-btn-gold av-w-full"
                      disabled={busy}
                      onClick={() => {
                        const note = speechNote.trim();
                        setSpeechNote("");
                        void runAction("finish_speech", { summary: note });
                      }}
                    >
                      <span>✓ 群聊发言完毕 · 移交圣剑麦序</span>
                    </button>
                  </>
                ) : (
                  <div className="av-wait-note">{you.actionHint}</div>
                )}
              </>
            )}

            {/* 4. TEAM_VOTE ALTAR */}
            {view.phase === "TEAM_VOTE" && (
              <>
                <div className="av-altar-head">
                  <span className="av-altar-title">白黑火漆公投（齐票后公开票型）</span>
                  <span className="av-altar-sub">
                    车队：{(view.proposedTeamSeats ?? []).map((s) => `#${s}`).join("、")}
                  </span>
                </div>

                {you.seated && !you.hasVotedTeam ? (
                  <div className="av-tactile-pair">
                    <button
                      type="button"
                      className="av-token-card white-seal"
                      disabled={busy}
                      onClick={() => void runAction("vote_team", { approve: true })}
                    >
                      <WhiteVoteSealIcon className="av-svg-md" />
                      <span className="av-token-title">白章 · 赞成发车</span>
                      <span className="av-token-sub">批准本轮车队远征</span>
                    </button>

                    <button
                      type="button"
                      className="av-token-card black-seal"
                      disabled={busy}
                      onClick={() => void runAction("vote_team", { approve: false })}
                    >
                      <BlackVoteSealIcon className="av-svg-md" />
                      <span className="av-token-title">黑章 · 反对流票</span>
                      <span className="av-token-sub">否决车队移交队长</span>
                    </button>
                  </div>
                ) : (
                  <div className="av-wait-note">
                    {you.hasVotedTeam
                      ? `✓ 你已暗扣【${you.myTeamVote ? "⚪ 白章赞成" : "⚫ 黑章反对"}】，待全员落章后同步亮票。`
                      : you.actionHint}
                  </div>
                )}
              </>
            )}

            {/* 5. QUEST_EXECUTION ALTAR */}
            {view.phase === "QUEST_EXECUTION" && (
              <>
                <div className="av-altar-head">
                  <span className="av-altar-title">
                    <HolyGrailIcon className="av-svg-sm" />
                    <span>圣杯远征暗票（洗牌匿名开票）</span>
                  </span>
                  <span className="av-badge gold">
                    已出牌 {view.questSubmittedCount ?? 0}/
                    {view.questRequiredCards ?? teamSizeNeeded}
                  </span>
                </div>

                {you.onQuestTeam && !you.hasSubmittedQuest ? (
                  <div className="av-tactile-pair">
                    <button
                      type="button"
                      className="av-token-card white-seal"
                      disabled={busy}
                      onClick={() => void runAction("submit_quest_card", { success: true })}
                    >
                      <HolyGrailIcon className="av-svg-md" />
                      <span className="av-token-title">圣杯成功卡</span>
                      <span className="av-token-sub">护佑远征圆满成功</span>
                    </button>

                    <button
                      type="button"
                      className="av-token-card black-seal"
                      disabled={busy || you.alignment === "GOOD"}
                      title={
                        you.alignment === "GOOD"
                          ? "亚瑟蓝方骑士受誓约约束，严禁打出任务破坏卡"
                          : "红方暗影可打出破坏卡炸毁本轮远征"
                      }
                      onClick={() => void runAction("submit_quest_card", { success: false })}
                    >
                      <SabotageDaggerIcon className="av-svg-md" />
                      <span className="av-token-title">
                        {you.alignment === "GOOD" ? "破坏卡 (誓约锁)" : "暗影破坏卡"}
                      </span>
                      <span className="av-token-sub">
                        {you.alignment === "GOOD" ? "蓝方好人禁止投失败" : "暗中炸毁本轮任务"}
                      </span>
                    </button>
                  </div>
                ) : (
                  <div className="av-wait-note">{you.actionHint}</div>
                )}
              </>
            )}

            {/* 6. ASSASSINATION ALTAR */}
            {view.phase === "ASSASSINATION" && (
              <>
                <div className="av-altar-head">
                  <span className="av-altar-title" style={{ color: "var(--evil-bright)" }}>
                    <SabotageDaggerIcon className="av-svg-sm" />
                    <span>蓝方已夺三圣杯 · 刺客亮刀时刻</span>
                  </span>
                </div>

                {you.role === "ASSASSIN" ? (
                  <>
                    <div style={{ fontSize: "12px", color: "var(--text-secondary)" }}>
                      请在圆桌点击最像真【梅林】的蓝方骑士，刺中即可逆转翻盘：
                    </div>
                    <button
                      type="button"
                      className="av-btn av-btn-crimson av-w-full"
                      disabled={busy || assassinTarget <= 0}
                      onClick={() => void runAction("assassinate", { targetSeat: assassinTarget })}
                    >
                      <SabotageDaggerIcon className="av-svg-sm" />
                      <span>
                        {assassinTarget > 0
                          ? `一剑封喉 · 拍刀刺杀 ${seatNameByNum(assassinTarget)}`
                          : "请先在圆桌点击锁定的刺杀座位"}
                      </span>
                    </button>
                  </>
                ) : (
                  <div className="av-wait-note">{you.actionHint}</div>
                )}
              </>
            )}

            {/* 7. GAME_OVER ALTAR */}
            {view.phase === "GAME_OVER" && (
              <>
                <div className="av-altar-head">
                  <span
                    className="av-altar-title"
                    style={{
                      color: view.winner === "GOOD" ? "var(--good)" : "var(--evil)",
                      fontSize: "14px",
                    }}
                  >
                    {view.winner === "GOOD" ? (
                      <HolyGrailIcon className="av-svg-md" />
                    ) : (
                      <SabotageDaggerIcon className="av-svg-md" />
                    )}
                    <span>
                      {view.winner === "GOOD"
                        ? "亚瑟蓝方成功守护圣杯！"
                        : "莫德雷德红方颠覆卡美洛！"}
                    </span>
                  </span>
                  <span className="av-badge gold">底牌全公开</span>
                </div>

                <div style={{ fontSize: "12.5px", color: "var(--text-secondary)" }}>
                  {view.winSummary}
                </div>

                {view.assassination && (
                  <div className="av-wait-note">
                    🗡️ 终局拍刀：刺客 <b>#{view.assassination.assassinSeat}</b> 刺杀了{" "}
                    <b>{seatNameByNum(view.assassination.targetSeat)}</b>
                    {view.assassination.hitMerlin
                      ? "（正中真梅林，红方极限翻盘！）"
                      : "（未中梅林，蓝方护主成功！）"}
                  </div>
                )}

                <button
                  type="button"
                  className="av-btn av-btn-gold av-w-full"
                  disabled={busy}
                  onClick={() => void runAction("reset_lobby", {})}
                >
                  <span>✦ 原地重开下一局（保留圆桌席位）</span>
                </button>
              </>
            )}
          </section>

          {/* C. Always-Visible Inline Deduction Matrix on Medium/Wide Panes (>= 720px) */}
          <section className="av-inline-ledger" aria-label="公开票型推演矩阵">
            <LedgerPanel
              view={view}
              reversedVotes={reversedVotes}
              ledgerTab={ledgerTab}
              setLedgerTab={setLedgerTab}
            />
          </section>
        </aside>
      </div>

      {/* 3. Compact-Pane Collapsible Bottom Ledger Dock (hidden on >= 720px where inline matrix is open) */}
      <footer className="av-ledger-dock">
        <button
          type="button"
          className="av-ledger-trigger"
          onClick={() => setLedgerOpen((v) => !v)}
          aria-expanded={ledgerOpen}
        >
          <span className="av-ledger-latest">
            <ScrollLedgerIcon className="av-svg-sm" />
            <span>{latestEvent}</span>
          </span>
          <span className="av-ledger-pill">
            <span>票型表 ({view.voteHistory.length})</span>
            <ChevronIcon open={ledgerOpen} className="av-svg-sm" />
          </span>
        </button>

        {ledgerOpen && (
          <div className="av-ledger-body">
            <LedgerPanel
              view={view}
              reversedVotes={reversedVotes}
              ledgerTab={ledgerTab}
              setLedgerTab={setLedgerTab}
            />
          </div>
        )}
      </footer>
    </main>
  );
}
