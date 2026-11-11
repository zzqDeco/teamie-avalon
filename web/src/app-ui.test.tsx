// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import type { PlayerView } from "./types";

vi.mock("./api", () => ({
  fetchMyView: vi.fn(),
  postGameAction: vi.fn(),
  subscribeViewStream: vi.fn(() => () => {}),
}));

import { fetchMyView, postGameAction } from "./api";

function makeSampleView(overrides: Partial<PlayerView> = {}): PlayerView {
  return {
    workspaceId: "ws-1",
    conversationId: "conv-1",
    gameId: "game-1",
    eventSeq: 3,
    phase: "TEAM_PROPOSAL",
    phaseLabel: "队长组队",
    preset: "5p",
    targetPlayerCount: 5,
    goodCount: 3,
    evilCount: 2,
    roleOptions: {
      merlin: true,
      percival: true,
      assassin: true,
      morgana: true,
      mordred: false,
      oberon: false,
    },
    phaseTimeoutSec: 90,
    phaseDeadlineUnixMs: Date.now() + 65_000,
    questNumber: 1,
    proposalAttempt: 1,
    rejectedCount: 0,
    isHammerProposal: false,
    leaderSeat: 1,
    currentTeamSize: 2,
    questTrack: [
      { questNumber: 1, teamSize: 2, failsRequired: 1, twoFailsRequired: false, status: "ACTIVE" },
      { questNumber: 2, teamSize: 3, failsRequired: 1, twoFailsRequired: false, status: "PENDING" },
      { questNumber: 3, teamSize: 2, failsRequired: 1, twoFailsRequired: false, status: "PENDING" },
      { questNumber: 4, teamSize: 3, failsRequired: 1, twoFailsRequired: false, status: "PENDING" },
      { questNumber: 5, teamSize: 3, failsRequired: 1, twoFailsRequired: false, status: "PENDING" },
    ],
    completedQuests: [],
    voteHistory: [
      {
        questNumber: 1,
        attemptNumber: 1,
        leaderSeat: 1,
        teamSeats: [1, 3],
        votes: [
          { seat: 1, displayName: "亚瑟", approve: true },
          { seat: 2, displayName: "莫甘娜", approve: false },
          { seat: 3, displayName: "派西维尔", approve: true },
          { seat: 4, displayName: "刺客", approve: false },
          { seat: 5, displayName: "忠臣", approve: true },
        ],
        approveCount: 3,
        rejectCount: 2,
        approved: true,
      },
    ],
    successQuestCount: 0,
    failQuestCount: 0,
    seats: [
      {
        seat: 1,
        userId: "u-1",
        displayName: "亚瑟",
        isHost: true,
        isLeader: true,
        isCurrentSpeaker: false,
        onProposedTeam: false,
        hasSpokenThisTurn: false,
        hasVotedTeam: false,
      },
      {
        seat: 2,
        userId: "u-2",
        displayName: "兰斯洛特",
        isHost: false,
        isLeader: false,
        isCurrentSpeaker: false,
        onProposedTeam: false,
        hasSpokenThisTurn: false,
        hasVotedTeam: false,
      },
      {
        seat: 3,
        userId: "u-3",
        displayName: "高文",
        isHost: false,
        isLeader: false,
        isCurrentSpeaker: false,
        onProposedTeam: false,
        hasSpokenThisTurn: false,
        hasVotedTeam: false,
      },
      {
        seat: 4,
        userId: "u-4",
        displayName: "崔斯坦",
        isHost: false,
        isLeader: false,
        isCurrentSpeaker: false,
        onProposedTeam: false,
        hasSpokenThisTurn: false,
        hasVotedTeam: false,
      },
      {
        seat: 5,
        userId: "u-5",
        displayName: "加拉哈德",
        isHost: false,
        isLeader: false,
        isCurrentSpeaker: false,
        onProposedTeam: false,
        hasSpokenThisTurn: false,
        hasVotedTeam: false,
      },
    ],
    you: {
      userId: "u-1",
      displayName: "亚瑟",
      seated: true,
      seat: 1,
      isHost: true,
      isLeader: true,
      role: "MERLIN",
      roleName: "梅林",
      alignment: "GOOD",
      privateVision: {
        visibleEvilSeats: [2, 4],
        visionSummary: "你看到了场上除莫德雷德外的红方坏人座位。",
      },
      canActNow: true,
    },
    publicChatPolicy: {
      allowed: false,
      phaseMode: "LEADER_PROPOSING",
      guidance: "队长提名车队期间请保持安静，待进入发言轮后再依次陈词。",
    },
    recentEvents: ["第 1 轮任务开启，由 #1 亚瑟担任首任队长。"],
    ...overrides,
  };
}

describe("Camelot Royal Avalon Drawer UI", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    vi.clearAllMocks();
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it("renders Camelot handshake screen, projects Fog-of-War clues & conceals cleanly, drafts team on seats, and opens Vote Matrix", async () => {
    const sample = makeSampleView();
    vi.mocked(fetchMyView).mockResolvedValue(sample);
    vi.mocked(postGameAction).mockResolvedValue({
      ok: true,
      eventSeq: 4,
      message: "ok",
      phase: "PROPOSAL_SPEECH",
      publicChatPolicy: sample.publicChatPolicy,
      view: sample,
    });

    await act(async () => {
      root.render(<App />);
    });

    // 1. Waiting handshake screen
    expect(container.textContent).toContain("阿瓦隆 · 卡美洛圆桌裁判台");

    // 2. Dispatch Teamie host init message
    await act(async () => {
      window.dispatchEvent(
        new MessageEvent("message", {
          source: window.parent,
          origin: "https://120.25.210.202",
          data: {
            type: "teamie:mcp-app:init",
            payload: {
              appId: "app-avalon",
              slug: "avalon",
              name: "阿瓦隆圆桌裁判台",
              webUrl: "https://120.25.210.202:9444/",
              workspaceId: "ws-1",
              conversationId: "conv-1",
              token: "jwt-token-1",
              expiresAt: new Date(Date.now() + 3600_000).toISOString(),
              user: { id: "u-1", displayName: "亚瑟" },
            },
          },
        }),
      );
    });

    // 3. Tarot Identity & seat-level Fog-of-War projection visible when revealed
    expect(container.textContent).toContain("亚瑟蓝方 · 梅林");
    expect(container.textContent).toContain("红方暗影");

    // Click Tarot card to veil identity -> hides both plaque role & seat Fog-of-War badges
    const tarotBtn = container.querySelector(".av-tarot-card") as HTMLButtonElement;
    expect(tarotBtn).toBeTruthy();
    await act(async () => {
      tarotBtn.click();
    });
    expect(container.textContent).toContain("圣剑密卷已暗置防窥");
    expect(container.textContent).not.toContain("红方暗影");

    // Flip back open
    await act(async () => {
      tarotBtn.click();
    });
    expect(container.textContent).toContain("红方暗影");

    // 4. Direct-on-seat team drafting (#1 + #3) on Circular Round Table -> submit propose_team
    const seatCards = Array.from(container.querySelectorAll(".av-seat-card")) as HTMLButtonElement[];
    expect(seatCards).toHaveLength(5);
    // Verify polar Round Table CSS custom properties are assigned clockwise from 12 o'clock (#1 at 50%, 12%)
    expect(seatCards[0].style.getPropertyValue("--seat-x")).toBe("50%");
    expect(seatCards[0].style.getPropertyValue("--seat-y")).toBe("12%");

    await act(async () => {
      seatCards[0].click();
      seatCards[2].click();
    });
    expect(container.textContent).toContain("确认提名远征车队（2/2）");

    const proposeBtn = Array.from(container.querySelectorAll("button")).find((b) =>
      b.textContent?.includes("确认提名远征车队（2/2）"),
    ) as HTMLButtonElement;
    expect(proposeBtn.disabled).toBe(false);

    await act(async () => {
      proposeBtn.click();
    });
    expect(postGameAction).toHaveBeenCalledWith(
      "jwt-token-1",
      "propose_team",
      { teamSeats: [1, 3] },
      expect.any(Function),
    );

    // 5. Verify Seat-Aligned Vote Deduction Matrix (always rendered inline + toggleable bottom dock)
    const inlineMatrix = container.querySelector(".av-inline-ledger table.av-matrix-table");
    expect(inlineMatrix).toBeTruthy();
    expect(inlineMatrix?.textContent).toContain("发车 3:2");
    expect(inlineMatrix?.textContent).toContain("🛡️赞");
    expect(inlineMatrix?.textContent).toContain("反");

    const dockBtn = container.querySelector(".av-ledger-trigger") as HTMLButtonElement;
    await act(async () => {
      dockBtn.click();
    });
    expect(container.querySelectorAll("table.av-matrix-table")).toHaveLength(2);
  });
});
