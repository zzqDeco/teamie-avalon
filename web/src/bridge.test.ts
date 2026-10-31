// @vitest-environment jsdom
import { describe, expect, it, vi } from "vitest";
import { TeamieHostBridge } from "./bridge";

describe("TeamieHostBridge postMessage protocol", () => {
  it("posts ready to *, locks hostOrigin on first init, ignores spoofed source/origin, and never touches localStorage", () => {
    const parentPostMessage = vi.fn();
    const fakeParent = { postMessage: parentPostMessage } as unknown as Window;
    const updates: unknown[] = [];
    const setItemSpy = vi.spyOn(Storage.prototype, "setItem");

    const nowMs = Date.parse("2026-10-31T15:00:00Z");
    const bridge = new TeamieHostBridge({
      win: window,
      parentWin: fakeParent,
      now: () => nowMs,
      onUpdate: (st) => updates.push(st),
    });

    bridge.start();
    expect(parentPostMessage).toHaveBeenCalledWith(
      { type: "teamie:mcp-app:ready" },
      "*",
    );

    // Ignore message from wrong source
    window.dispatchEvent(
      new MessageEvent("message", {
        source: window,
        origin: "https://evil.example.com",
        data: {
          type: "teamie:mcp-app:init",
          payload: {
            appId: "app-1",
            workspaceId: "ws-1",
            conversationId: "conv-1",
            token: "evil-tok",
            expiresAt: "2026-10-31T16:00:00Z",
          },
        },
      }),
    );
    expect(updates).toHaveLength(0);

    // Valid init from window.parent locks hostOrigin
    window.dispatchEvent(
      new MessageEvent("message", {
        source: fakeParent,
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
            token: "valid-jwt-1",
            expiresAt: "2026-10-31T16:00:00Z",
            user: { id: "u-1", displayName: "Arthur" },
          },
        },
      }),
    );

    expect(updates).toHaveLength(1);
    expect(bridge.getState().hostOrigin).toBe("https://120.25.210.202");
    expect(bridge.getState().context?.token).toBe("valid-jwt-1");
    expect(setItemSpy).not.toHaveBeenCalled();

    // Subsequent message from another origin (even with fakeParent) MUST be rejected
    window.dispatchEvent(
      new MessageEvent("message", {
        source: fakeParent,
        origin: "https://other-origin.example.com",
        data: {
          type: "teamie:mcp-app:init",
          payload: {
            appId: "app-avalon",
            workspaceId: "ws-1",
            conversationId: "conv-1",
            token: "hijacked-jwt",
            expiresAt: "2026-10-31T16:00:00Z",
          },
        },
      }),
    );
    expect(bridge.getState().context?.token).toBe("valid-jwt-1");

    // Silent refresh sends strictly to locked hostOrigin
    expect(bridge.requestTokenRefresh(true)).toBe(true);
    expect(parentPostMessage).toHaveBeenLastCalledWith(
      { type: "teamie:mcp-app:refresh-token" },
      "https://120.25.210.202",
    );

    bridge.stop();
    setItemSpy.mockRestore();
  });

  it("immediately triggers refresh-token when expiresAt is within 5 minutes", () => {
    const parentPostMessage = vi.fn();
    const fakeParent = { postMessage: parentPostMessage } as unknown as Window;
    const nowMs = Date.parse("2026-10-31T15:56:00Z");

    const bridge = new TeamieHostBridge({
      win: window,
      parentWin: fakeParent,
      now: () => nowMs,
      onUpdate: () => {},
    });

    bridge.start();
    window.dispatchEvent(
      new MessageEvent("message", {
        source: fakeParent,
        origin: "https://120.25.210.202",
        data: {
          type: "teamie:mcp-app:init",
          payload: {
            appId: "app-avalon",
            workspaceId: "ws-1",
            conversationId: "conv-1",
            token: "expiring-soon-jwt",
            expiresAt: "2026-10-31T16:00:00Z", // 4 minutes left (< 5m threshold)
          },
        },
      }),
    );

    expect(parentPostMessage).toHaveBeenCalledWith(
      { type: "teamie:mcp-app:refresh-token" },
      "https://120.25.210.202",
    );
    bridge.stop();
  });
});
