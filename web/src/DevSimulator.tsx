import { useCallback, useEffect, useRef, useState } from "react";
import type { TeamieInitPayload } from "./types";

interface DevPlayerSession {
  slot: number;
  userId: string;
  displayName: string;
  token: string;
  expiresAt: string;
}

interface DevTokensResp {
  appId: string;
  workspaceId: string;
  conversationId: string;
  preset: string;
  players: DevPlayerSession[];
}

export function DevSimulator() {
  const [preset, setPreset] = useState("5p");
  const [data, setData] = useState<DevTokensResp | null>(null);
  const [loading, setLoading] = useState(false);
  const [errMsg, setErrMsg] = useState<string | null>(null);

  const iframeRefs = useRef<Map<number, HTMLIFrameElement>>(new Map());
  const sessionsRef = useRef<DevTokensResp | null>(null);

  const pushInitToSlot = useCallback((slot: number, sourceData: DevTokensResp | null) => {
    if (!sourceData) return;
    const iframe = iframeRefs.current.get(slot);
    const p = sourceData.players.find((item) => item.slot === slot);
    if (!iframe?.contentWindow || !p) return;

    const payload: TeamieInitPayload = {
      appId: sourceData.appId,
      slug: "avalon",
      name: "阿瓦隆圆桌裁判台 (Dev)",
      webUrl: `${window.location.origin}/`,
      mcpUrl: `${window.location.origin}/mcp`,
      token: p.token,
      expiresAt: p.expiresAt,
      workspaceId: sourceData.workspaceId,
      conversationId: sourceData.conversationId,
      user: {
        id: p.userId,
        displayName: p.displayName,
      },
    };
    iframe.contentWindow.postMessage(
      { type: "teamie:mcp-app:init", payload },
      window.location.origin,
    );
  }, []);

  const loadTokens = useCallback(
    async (nextPreset: string, quickAction = "") => {
      setLoading(true);
      setErrMsg(null);
      try {
        const resp = await fetch("/api/dev/tokens", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            preset: nextPreset,
            quickAction,
          }),
        });
        if (!resp.ok) {
          throw new Error(`HTTP ${resp.status} (/dev 仅在 DEV_SIMULATOR=true 时启用)`);
        }
        const json = (await resp.json()) as DevTokensResp;
        sessionsRef.current = json;
        setData(json);
        for (const p of json.players) {
          pushInitToSlot(p.slot, json);
        }
      } catch (e: unknown) {
        setErrMsg(e instanceof Error ? e.message : "加载模拟器失败");
      } finally {
        setLoading(false);
      }
    },
    [pushInitToSlot],
  );

  useEffect(() => {
    void loadTokens(preset, "seat_all");
  }, [preset, loadTokens]);

  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      const msg = event.data as { type?: string } | null;
      if (!msg?.type) return;
      if (msg.type !== "teamie:mcp-app:ready" && msg.type !== "teamie:mcp-app:refresh-token") {
        return;
      }
      const cur = sessionsRef.current;
      if (!cur) return;
      for (const [slot, iframe] of iframeRefs.current.entries()) {
        if (iframe.contentWindow === event.source) {
          if (msg.type === "teamie:mcp-app:refresh-token") {
            void loadTokens(preset, "");
          } else {
            pushInitToSlot(slot, cur);
          }
          break;
        }
      }
    };
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, [preset, loadTokens, pushInitToSlot]);

  return (
    <div className="dev-workbench">
      <header className="dev-toolbar">
        <div className="dev-toolbar-left">
          <h1>⚔️ 阿瓦隆本地多视角联调台 (/dev)</h1>
          <span className="faint-text">
            真实跨抽屉 <code>postMessage</code> 握手 + 对称 HS256 JWT + 非对称迷雾验证
          </span>
        </div>
        <div className="dev-toolbar-controls">
          <select
            className="select-input"
            value={preset}
            disabled={loading}
            onChange={(e) => setPreset(e.target.value)}
          >
            <option value="5p">5人标准局 (3蓝2红)</option>
            <option value="6p">6人标准局 (4蓝2红)</option>
            <option value="7p_oberon">7人奥伯伦局 (Q4保护轮)</option>
            <option value="7p_mordred">7人莫德雷德局 (Q4保护轮)</option>
            <option value="8p">8人局 (5蓝3红)</option>
            <option value="10p">10人全神局 (6蓝4红)</option>
          </select>
          <button
            type="button"
            className="btn btn-secondary"
            disabled={loading}
            onClick={() => void loadTokens(preset, "seat_all")}
          >
            🪑 全员入座
          </button>
          <button
            type="button"
            className="btn btn-primary"
            disabled={loading}
            onClick={() => void loadTokens(preset, "start_game")}
          >
            ⚔️ 一键发牌开局
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            disabled={loading}
            onClick={() => void loadTokens(preset, "reset_lobby")}
          >
            🔄 重置回大厅
          </button>
        </div>
      </header>

      {errMsg && <div className="error-banner">{errMsg}</div>}

      <div className="dev-frames-strip">
        {(data?.players ?? []).map((p) => (
          <div key={p.slot} className="dev-frame-column">
            <div className="dev-frame-title">
              <span>视角 #{p.slot}</span>
              <b>{p.displayName}</b>
            </div>
            <iframe
              title={`avalon-seat-${p.slot}`}
              src={`/?devSlot=${p.slot}`}
              className="dev-drawer-iframe"
              ref={(el) => {
                if (el) {
                  iframeRefs.current.set(p.slot, el);
                } else {
                  iframeRefs.current.delete(p.slot);
                }
              }}
            />
          </div>
        ))}
      </div>
    </div>
  );
}
