import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { fetchMyView, postGameAction, subscribeViewStream } from "./api";
import { TeamieHostBridge, type BridgeState } from "./bridge";
import type { PlayerView, RoleOptions } from "./types";

const PRESET_OPTIONS: { value: string; label: string }[] = [
  { value: "5p", label: "5人标准 (梅派忠 vs 莫刺)" },
  { value: "6p", label: "6人标准 (梅派2忠 vs 莫刺)" },
  { value: "7p_oberon", label: "7人奥伯伦 (4蓝 vs 莫刺奥)" },
  { value: "7p_mordred", label: "7人莫德雷德 (4蓝 vs 莫刺黑)" },
  { value: "8p", label: "8人标准 (5蓝 vs 莫刺黑)" },
  { value: "9p", label: "9人标准 (6蓝 vs 莫刺黑)" },
  { value: "10p", label: "10人全神 (6蓝 vs 莫刺黑奥)" },
];

const ROLE_SWITCH_LABELS: { key: keyof RoleOptions; label: string; side: "good" | "evil" }[] = [
  { key: "merlin", label: "梅林", side: "good" },
  { key: "percival", label: "派西维尔", side: "good" },
  { key: "assassin", label: "刺客", side: "evil" },
  { key: "morgana", label: "莫甘娜", side: "evil" },
  { key: "mordred", label: "莫德雷德", side: "evil" },
  { key: "oberon", label: "奥伯伦", side: "evil" },
];

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

  // Local interactive selections
  const [selectedTeam, setSelectedTeam] = useState<number[]>([]);
  const [speechNote, setSpeechNote] = useState("");
  const [assassinTarget, setAssassinTarget] = useState<number>(0);
  const [ledgerTab, setLedgerTab] = useState<"votes" | "quests" | "events">("votes");
  const [remainSec, setRemainSec] = useState<number | null>(null);

  const bridgeRef = useRef<TeamieHostBridge | null>(null);

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
          setErrorMsg(err instanceof Error ? err.message : "加载状态失败");
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

  // Reset team selection when quest/attempt changes
  useEffect(() => {
    setSelectedTeam([]);
    setAssassinTarget(0);
  }, [view?.gameId, view?.questNumber, view?.proposalAttempt, view?.phase]);

  // Local 1s visual countdown computed purely from phaseDeadlineUnixMs (zero server tick traffic)
  useEffect(() => {
    const deadline = view?.phaseDeadlineUnixMs;
    if (!deadline) {
      setRemainSec(null);
      return;
    }
    const update = () => {
      const diff = Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
      setRemainSec(diff);
    };
    update();
    const id = window.setInterval(update, 1000);
    return () => window.clearInterval(id);
  }, [view?.phaseDeadlineUnixMs]);

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
        return [...prev.slice(1), seat];
      }
      return [...prev, seat].sort((a, b) => a - b);
    });
  };

  const reversedVotes = useMemo(() => {
    if (!view?.voteHistory) return [];
    return [...view.voteHistory].reverse();
  }, [view?.voteHistory]);

  if (!bridgeState.ready || !bridgeState.context) {
    return (
      <div className="drawer-shell">
        <div className="card empty-handshake">
          <div className="crest-badge">⚔️ 阿瓦隆圆桌裁判台</div>
          <p className="muted-text">
            正在等待 Teamie 群聊宿主下发安全会话凭证（<code>teamie:mcp-app:init</code>）…
          </p>
          <p className="faint-text">
            请在安装了本应用的 Teamie 群聊右侧栏中打开，支持 5–10 名人类与 Eino Agent 同桌推理博弈。
          </p>
        </div>
      </div>
    );
  }

  if (!view) {
    return (
      <div className="drawer-shell">
        <div className="card empty-handshake">
          <div className="crest-badge">⚔️ 阿瓦隆圆桌裁判台</div>
          <p className="muted-text">正在连接圆桌状态机…</p>
          {errorMsg && <div className="error-banner">{errorMsg}</div>}
        </div>
      </div>
    );
  }

  const { you } = view;
  const teamSizeNeeded = view.currentTeamSize ?? 2;
  const knownEvilTeammates = you.privateVision?.evilTeammateSeats ?? [];

  return (
    <div className="drawer-shell">
      {/* 1. Compact Header Bar */}
      <header className="top-bar card">
        <div className="top-bar-row">
          <div className="brand-title">
            <span className={`live-dot ${sseConnected ? "online" : "offline"}`} title={sseConnected ? "实时连接正常" : "重连中"} />
            <span>阿瓦隆圆桌</span>
            <span className={`phase-pill phase-${view.phase.toLowerCase()}`}>{view.phaseLabel}</span>
          </div>
          <div className="top-bar-actions">
            {remainSec !== null && view.phase !== "LOBBY" && view.phase !== "GAME_OVER" && (
              <span className={`timer-pill ${remainSec <= 15 ? "urgent" : ""}`}>{remainSec}s</span>
            )}
            {you.role && (
              <button
                type="button"
                className="ghost-mini-btn"
                onClick={() => setHideIdentity((v) => !v)}
              >
                {hideIdentity ? "展开底牌" : "遮罩底牌"}
              </button>
            )}
          </div>
        </div>
        <div className="sub-meta-row">
          <span>
            {you.seated ? `你的座位：#${you.seat} ${you.displayName}` : `旁观：${you.displayName}`}
          </span>
          <span>
            {view.goodCount}蓝 vs {view.evilCount}红
          </span>
        </div>
      </header>

      {/* 2. 5-Quest Track & 5-Reject Track */}
      <section className="card track-card" aria-label="任务与流票进度">
        <div className="section-label-row">
          <span className="section-title">5 轮任务进度轨</span>
          <span className="score-summary">
            <b className="good-txt">蓝 {view.successQuestCount}</b> : <b className="evil-txt">红 {view.failQuestCount}</b>
          </span>
        </div>
        <div className="quest-track">
          {view.questTrack.map((slot) => (
            <div
              key={slot.questNumber}
              className={`quest-node status-${slot.status.toLowerCase()}`}
              title={
                slot.twoFailsRequired
                  ? `第 ${slot.questNumber} 轮：需 ${slot.teamSize} 人上车（保护轮：需至少 2 张失败票才算失败）`
                  : `第 ${slot.questNumber} 轮：需 ${slot.teamSize} 人上车`
              }
            >
              <div className="quest-node-top">
                <span className="q-idx">Q{slot.questNumber}</span>
                {slot.twoFailsRequired && <span className="q-protect">2败!</span>}
              </div>
              <div className="q-size">
                {slot.status === "SUCCEEDED"
                  ? "🛡️"
                  : slot.status === "FAILED"
                    ? "🗡️"
                    : `${slot.teamSize}人`}
              </div>
              <div className="q-tally">
                {slot.successCount !== undefined && slot.failCount !== undefined
                  ? `${slot.successCount}成/${slot.failCount}败`
                  : `${slot.teamSize}人车`}
              </div>
            </div>
          ))}
        </div>

        <div className="reject-track-wrap">
          <div className="reject-header">
            <span>流票计数轨（连否 5 次红方直胜）</span>
            <span className={view.isHammerProposal ? "evil-txt bold" : "faint-text"}>
              {view.rejectedCount} / 5
            </span>
          </div>
          <div className="reject-pips">
            {[1, 2, 3, 4, 5].map((idx) => (
              <div
                key={idx}
                className={`reject-pip ${idx <= view.rejectedCount ? "filled" : ""} ${
                  idx === 5 ? "hammer-slot" : ""
                }`}
              >
                {idx === 5 ? "5·尾车" : idx}
              </div>
            ))}
          </div>
          {view.isHammerProposal && (
            <div className="hammer-alert">
              ⚠️ 当前为第 5 任尾置位队长发车（Hammer）！若本车再被公投否决，红方将直接获胜！
            </div>
          )}
        </div>
      </section>

      {/* Error Banner */}
      {errorMsg && <div className="error-banner">{errorMsg}</div>}

      {/* 3. Asymmetric Private Vision Card */}
      {you.role && !hideIdentity && (
        <section
          className={`card vision-card ${you.alignment === "GOOD" ? "vision-good" : "vision-evil"}`}
          aria-label="非对称暗牌视野"
        >
          <div className="vision-header">
            <span className="role-badge">
              {you.alignment === "GOOD" ? "🔵 蓝方好人" : "🔴 红方坏人"} · {you.roleName}
            </span>
          </div>
          {you.privateVision && (
            <>
              {you.privateVision.visibleEvilSeats && you.privateVision.visibleEvilSeats.length > 0 && (
                <div className="vision-chips-row">
                  <span className="chip-label">可见红方(不含莫德雷德)：</span>
                  {you.privateVision.visibleEvilSeats.map((s) => (
                    <span key={s} className="seat-chip evil-chip">
                      #{s}
                    </span>
                  ))}
                </div>
              )}
              {you.privateVision.merlinCandidateSeats &&
                you.privateVision.merlinCandidateSeats.length > 0 && (
                  <div className="vision-chips-row">
                    <span className="chip-label">梅林候选(真假已乱序)：</span>
                    {you.privateVision.merlinCandidateSeats.map((s) => (
                      <span key={s} className="seat-chip mystic-chip">
                        #{s}
                      </span>
                    ))}
                  </div>
                )}
              {you.privateVision.evilTeammateSeats &&
                you.privateVision.evilTeammateSeats.length > 0 && (
                  <div className="vision-chips-row">
                    <span className="chip-label">红方同伴(不含奥伯伦)：</span>
                    {you.privateVision.evilTeammateSeats.map((s) => (
                      <span key={s} className="seat-chip evil-chip">
                        #{s}
                      </span>
                    ))}
                  </div>
                )}
              <p className="vision-note">{you.privateVision.visionSummary}</p>
            </>
          )}
        </section>
      )}

      {/* 4. Contextual Phase Action Card */}
      <section className="card action-card" aria-label="阶段操作台">
        <div className="policy-strip">
          <span className={`mic-badge ${view.publicChatPolicy.allowed ? "mic-open" : "mic-muted"}`}>
            {view.publicChatPolicy.allowed ? "💬 群聊可发言" : "🤫 群聊禁言期"}
          </span>
          <span className="policy-text">{view.publicChatPolicy.guidance}</span>
        </div>

        {/* LOBBY */}
        {view.phase === "LOBBY" && (
          <div className="action-stack">
            <div className="btn-row">
              {!you.seated ? (
                <button
                  type="button"
                  className="btn btn-primary full-width"
                  disabled={busy || view.seats.length >= 10}
                  onClick={() => void runAction("join_lobby", {})}
                >
                  🪑 入座圆桌（当前 {view.seats.length} 人）
                </button>
              ) : (
                <button
                  type="button"
                  className="btn btn-secondary"
                  disabled={busy}
                  onClick={() => void runAction("leave_lobby", {})}
                >
                  离开座位 (#{you.seat})
                </button>
              )}
              {you.seated && view.seats.length >= 5 && (you.isHost || view.seats.length === view.targetPlayerCount) && (
                <button
                  type="button"
                  className="btn btn-primary flex-grow"
                  disabled={busy}
                  onClick={() => void runAction("start_game", {})}
                >
                  ⚔️ 发牌开局 ({view.seats.length}人)
                </button>
              )}
            </div>

            {(you.isHost || view.seats.length === 0) && (
              <div className="host-config-box">
                <label className="field-label">
                  房主板子预设
                  <select
                    className="select-input"
                    value={view.preset === "custom" ? "custom" : view.preset}
                    disabled={busy}
                    onChange={(e) => {
                      if (e.target.value !== "custom") {
                        void runAction("configure_lobby", { preset: e.target.value });
                      }
                    }}
                  >
                    {PRESET_OPTIONS.map((opt) => (
                      <option key={opt.value} value={opt.value}>
                        {opt.label}
                      </option>
                    ))}
                    {view.preset === "custom" && <option value="custom">自定义神职配置</option>}
                  </select>
                </label>

                <div className="inline-controls">
                  <label className="field-label compact">
                    阶段超时
                    <select
                      className="select-input"
                      value={view.phaseTimeoutSec}
                      disabled={busy}
                      onChange={(e) =>
                        void runAction("configure_lobby", {
                          phaseTimeoutSec: Number(e.target.value),
                        })
                      }
                    >
                      <option value={60}>60 秒</option>
                      <option value={90}>90 秒</option>
                      <option value={120}>120 秒</option>
                      <option value={180}>180 秒</option>
                    </select>
                  </label>
                  <button
                    type="button"
                    className="ghost-mini-btn"
                    onClick={() => setShowRoleCustomize((s) => !s)}
                  >
                    {showRoleCustomize ? "收起角色微调" : "微调特殊角色"}
                  </button>
                </div>

                {showRoleCustomize && (
                  <div className="role-toggle-grid">
                    {ROLE_SWITCH_LABELS.map((item) => {
                      const active = view.roleOptions[item.key];
                      return (
                        <button
                          key={item.key}
                          type="button"
                          disabled={busy}
                          className={`role-toggle-pill ${active ? `on-${item.side}` : ""}`}
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
                          {active ? "✓ " : "+ "}
                          {item.label}
                        </button>
                      );
                    })}
                    <div className="faint-text full-span">
                      剩余席位自动补齐为【亚瑟忠臣】与【红方爪牙】。
                    </div>
                  </div>
                )}
              </div>
            )}

            {view.seats.length < 5 && (
              <p className="faint-text">
                需至少 5 名玩家（支持 5–10 人人类与 Agent 混排）入座后发牌开局。
              </p>
            )}
          </div>
        )}

        {/* TEAM_PROPOSAL */}
        {view.phase === "TEAM_PROPOSAL" && (
          <div className="action-stack">
            {you.isLeader ? (
              <>
                <div className="prompt-title">
                  👑 你是第 {view.questNumber} 轮队长：请选择 <b>{teamSizeNeeded}</b> 名车队成员
                </div>
                <div className="seat-picker-grid">
                  {view.seats.map((s) => {
                    const picked = selectedTeam.includes(s.seat);
                    return (
                      <button
                        key={s.seat}
                        type="button"
                        className={`picker-chip ${picked ? "picked" : ""}`}
                        onClick={() => toggleTeamSeat(s.seat, teamSizeNeeded)}
                      >
                        #{s.seat} {s.displayName}
                      </button>
                    );
                  })}
                </div>
                <button
                  type="button"
                  className="btn btn-primary full-width"
                  disabled={busy || selectedTeam.length !== teamSizeNeeded}
                  onClick={() => void runAction("propose_team", { teamSeats: selectedTeam })}
                >
                  🛡️ 提交提名车队 ({selectedTeam.length}/{teamSizeNeeded})
                </button>
              </>
            ) : (
              <p className="muted-text">{you.actionHint}</p>
            )}
          </div>
        )}

        {/* PROPOSAL_SPEECH */}
        {view.phase === "PROPOSAL_SPEECH" && (
          <div className="action-stack">
            <div className="proposed-banner">
              队长 <b>#{view.leaderSeat}</b> 提名车队：
              {(view.proposedTeamSeats ?? []).map((s) => (
                <span key={s} className="seat-chip team-chip">
                  #{s}
                </span>
              ))}
            </div>
            {view.speechOrder && (
              <div className="speech-order-bar">
                {view.speechOrder.map((s, idx) => {
                  const isCur = s === view.currentSpeakerSeat;
                  const done = (view.currentSpeechNotes ?? []).some((n) => n.seat === s);
                  return (
                    <span
                      key={s}
                      className={`speaker-step ${isCur ? "active" : done ? "done" : ""}`}
                    >
                      {isCur ? "🎙️" : done ? "✓" : ""}#{s}
                      {idx === view.speechOrder!.length - 1 ? "(总结)" : ""}
                    </span>
                  );
                })}
              </div>
            )}
            {you.isCurrentSpeaker ? (
              <div className="speaker-turn-box">
                <div className="prompt-title">
                  🎙️ 轮到你发言！请先在左侧群聊框发送观点，随后点击交麦：
                </div>
                <input
                  type="text"
                  className="text-input"
                  maxLength={80}
                  placeholder="可选：填写一句话态度备忘（如：保1踩4，赞成发车）"
                  value={speechNote}
                  onChange={(e) => setSpeechNote(e.target.value)}
                />
                <button
                  type="button"
                  className="btn btn-primary full-width"
                  disabled={busy}
                  onClick={() => {
                    const note = speechNote.trim();
                    setSpeechNote("");
                    void runAction("finish_speech", { summary: note });
                  }}
                >
                  ✅ 结束发言 · 交麦给下一位
                </button>
              </div>
            ) : (
              <p className="muted-text">{you.actionHint}</p>
            )}
          </div>
        )}

        {/* TEAM_VOTE */}
        {view.phase === "TEAM_VOTE" && (
          <div className="action-stack">
            <div className="proposed-banner">
              公投车队（队长 #{view.leaderSeat}）：
              {(view.proposedTeamSeats ?? []).map((s) => (
                <span key={s} className="seat-chip team-chip">
                  #{s}
                </span>
              ))}
            </div>
            {you.seated && !you.hasVotedTeam ? (
              <div className="btn-row">
                <button
                  type="button"
                  className="btn btn-approve flex-grow"
                  disabled={busy}
                  onClick={() => void runAction("vote_team", { approve: true })}
                >
                  👍 赞成发车
                </button>
                <button
                  type="button"
                  className="btn btn-reject flex-grow"
                  disabled={busy}
                  onClick={() => void runAction("vote_team", { approve: false })}
                >
                  👎 反对否决
                </button>
              </div>
            ) : (
              <p className="muted-text">{you.actionHint}</p>
            )}
          </div>
        )}

        {/* QUEST_EXECUTION */}
        {view.phase === "QUEST_EXECUTION" && (
          <div className="action-stack">
            <div className="proposed-banner">
              执行第 {view.questNumber} 轮任务（已出牌 {view.questSubmittedCount ?? 0}/
              {view.questRequiredCards ?? teamSizeNeeded}）：
              {(view.proposedTeamSeats ?? []).map((s) => (
                <span key={s} className="seat-chip team-chip">
                  #{s}
                </span>
              ))}
            </div>
            {you.onQuestTeam && !you.hasSubmittedQuest ? (
              <div className="btn-row">
                <button
                  type="button"
                  className="btn btn-approve flex-grow"
                  disabled={busy}
                  onClick={() => void runAction("submit_quest_card", { success: true })}
                >
                  🛡️ 任务成功
                </button>
                <button
                  type="button"
                  className="btn btn-danger flex-grow"
                  disabled={busy || you.alignment === "GOOD"}
                  title={
                    you.alignment === "GOOD"
                      ? "蓝方好人阵营严格禁止投任务失败票"
                      : "红方坏人可投失败票破坏任务"
                  }
                  onClick={() => void runAction("submit_quest_card", { success: false })}
                >
                  🗡️ {you.alignment === "GOOD" ? "破坏(蓝方禁用)" : "任务破坏"}
                </button>
              </div>
            ) : (
              <p className="muted-text">{you.actionHint}</p>
            )}
          </div>
        )}

        {/* ASSASSINATION */}
        {view.phase === "ASSASSINATION" && (
          <div className="action-stack">
            <div className="hammer-alert">
              🗡️ 蓝方已成 3 局！红方迎来最后翻盘机会：刺客拍刀指认梅林！
            </div>
            {you.role === "ASSASSIN" ? (
              <>
                <div className="seat-picker-grid">
                  {view.seats
                    .filter(
                      (s) => s.seat !== you.seat && !knownEvilTeammates.includes(s.seat),
                    )
                    .map((s) => (
                      <button
                        key={s.seat}
                        type="button"
                        className={`picker-chip ${
                          assassinTarget === s.seat ? "picked-danger" : ""
                        }`}
                        onClick={() => setAssassinTarget(s.seat)}
                      >
                        #{s.seat} {s.displayName}
                      </button>
                    ))}
                </div>
                <button
                  type="button"
                  className="btn btn-danger full-width"
                  disabled={busy || assassinTarget <= 0}
                  onClick={() =>
                    void runAction("assassinate", { targetSeat: assassinTarget })
                  }
                >
                  🗡️ 拍刀刺杀 #{assassinTarget || "?"}
                </button>
              </>
            ) : (
              <p className="muted-text">{you.actionHint}</p>
            )}
          </div>
        )}

        {/* GAME_OVER */}
        {view.phase === "GAME_OVER" && (
          <div className="action-stack">
            <div
              className={`gameover-banner ${
                view.winner === "GOOD" ? "win-good" : "win-evil"
              }`}
            >
              <div className="gameover-title">
                {view.winner === "GOOD" ? "🏆 亚瑟蓝方阵营获胜！" : "🔥 莫德雷德红方阵营获胜！"}
              </div>
              <div className="gameover-desc">{view.winSummary}</div>
            </div>
            <button
              type="button"
              className="btn btn-primary full-width"
              disabled={busy}
              onClick={() => void runAction("reset_lobby", {})}
            >
              🔄 原地重开下一局（保留座位）
            </button>
          </div>
        )}
      </section>

      {/* 5. Round Table Seats */}
      <section className="card seats-card" aria-label="圆桌席位">
        <div className="section-label-row">
          <span className="section-title">圆桌骑士席位 ({view.seats.length})</span>
          {view.leaderSeat ? <span className="faint-text">👑 队长 #{view.leaderSeat}</span> : null}
        </div>
        <div className="seats-grid">
          {view.seats.map((s) => (
            <div
              key={s.seat}
              className={`seat-tile ${s.seat === you.seat ? "is-me" : ""} ${
                s.onProposedTeam ? "on-team" : ""
              } ${s.isCurrentSpeaker ? "speaking" : ""}`}
            >
              <div className="seat-top">
                <span className="seat-num">#{s.seat}</span>
                <span className="seat-name" title={s.displayName}>
                  {s.displayName}
                </span>
              </div>
              <div className="seat-tags">
                {s.isHost && <span className="mini-tag">房主</span>}
                {s.isLeader && <span className="mini-tag crown">👑队长</span>}
                {s.onProposedTeam && <span className="mini-tag shield">🛡️车队</span>}
                {s.isCurrentSpeaker && <span className="mini-tag mic">🎙️持麦</span>}
                {view.phase === "TEAM_VOTE" && (
                  <span className={`mini-tag ${s.hasVotedTeam ? "voted" : ""}`}>
                    {s.hasVotedTeam ? "✓已投" : "待投"}
                  </span>
                )}
                {s.lastPublicVote !== undefined && view.phase !== "TEAM_VOTE" && (
                  <span className={`mini-tag ${s.lastPublicVote ? "vote-yes" : "vote-no"}`}>
                    上轮:{s.lastPublicVote ? "赞" : "反"}
                  </span>
                )}
                {s.revealedRole && (
                  <span
                    className={`mini-tag ${
                      s.revealedAlignment === "GOOD" ? "role-good" : "role-evil"
                    }`}
                  >
                    {s.revealedRoleName}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* 6. Public Vote History & Anonymous Quest Ledger */}
      <section className="card ledger-card" aria-label="公开票型与任务记录">
        <div className="tab-bar">
          <button
            type="button"
            className={`tab-btn ${ledgerTab === "votes" ? "active" : ""}`}
            onClick={() => setLedgerTab("votes")}
          >
            票型记录 ({view.voteHistory.length})
          </button>
          <button
            type="button"
            className={`tab-btn ${ledgerTab === "quests" ? "active" : ""}`}
            onClick={() => setLedgerTab("quests")}
          >
            任务开票 ({view.completedQuests.length})
          </button>
          <button
            type="button"
            className={`tab-btn ${ledgerTab === "events" ? "active" : ""}`}
            onClick={() => setLedgerTab("events")}
          >
            动态 ({view.recentEvents.length})
          </button>
        </div>

        {ledgerTab === "votes" && (
          <div className="ledger-list">
            {reversedVotes.length === 0 ? (
              <p className="faint-text">暂无公开公投记录，每次公投开票后将永久载入此处供盘逻辑。</p>
            ) : (
              reversedVotes.map((rec, i) => (
                <div key={`${rec.questNumber}-${rec.attemptNumber}-${i}`} className="vote-record-item">
                  <div className="vote-rec-head">
                    <span>
                      <b>
                        Q{rec.questNumber}·第{rec.attemptNumber}车
                      </b>{" "}
                      👑#{rec.leaderSeat} → [{rec.teamSeats.map((s) => `#${s}`).join(",")}]
                    </span>
                    <span className={rec.approved ? "good-txt bold" : "evil-txt bold"}>
                      {rec.approved ? `通过 (${rec.approveCount}:${rec.rejectCount})` : `否决 (${rec.approveCount}:${rec.rejectCount})`}
                    </span>
                  </div>
                  <div className="vote-ballots-wrap">
                    {rec.votes.map((v) => (
                      <span
                        key={v.seat}
                        className={`ballot-pill ${v.approve ? "yes" : "no"}`}
                        title={`${v.displayName}: ${v.approve ? "赞成" : "反对"}`}
                      >
                        #{v.seat}
                        {v.approve ? "赞" : "反"}
                      </span>
                    ))}
                  </div>
                </div>
              ))
            )}
          </div>
        )}

        {ledgerTab === "quests" && (
          <div className="ledger-list">
            {view.completedQuests.length === 0 ? (
              <p className="faint-text">暂无已完成任务记录（开票仅公开成败张数，绝对匿名）。</p>
            ) : (
              [...view.completedQuests].reverse().map((q) => (
                <div key={q.questNumber} className="vote-record-item">
                  <div className="vote-rec-head">
                    <span>
                      <b>第 {q.questNumber} 轮任务</b> 👑#{q.leaderSeat} · 车队 [
                      {q.teamSeats.map((s) => `#${s}`).join(",")}]
                    </span>
                    <span className={q.succeeded ? "good-txt bold" : "evil-txt bold"}>
                      {q.succeeded ? "🛡️ 成功" : "🗡️ 失败"}
                    </span>
                  </div>
                  <div className="faint-text">
                    匿名洗牌开票：成功卡 <b>{q.successCount}</b> 张 / 失败卡{" "}
                    <b>{q.failCount}</b> 张
                    {q.twoFailsRequired ? "（保护轮需2败）" : ""}
                  </div>
                </div>
              ))
            )}
          </div>
        )}

        {ledgerTab === "events" && (
          <div className="ledger-list">
            {view.recentEvents.length === 0 ? (
              <p className="faint-text">暂无圆桌动态。</p>
            ) : (
              [...view.recentEvents].reverse().map((ev, idx) => (
                <div key={idx} className="event-line">
                  {ev}
                </div>
              ))
            )}
          </div>
        )}
      </section>
    </div>
  );
}
