import { useCallback, useEffect, useRef, useState } from "react";
import { ExcaliburSealEmblem } from "./icons";
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

type DevLayoutMode = "stage" | "split" | "grid";

export function DevSimulator() {
  const initialParams =
    typeof window !== "undefined" ? new URLSearchParams(window.location.search) : null;
  const [preset, setPreset] = useState(() => initialParams?.get("preset") || "5p");
  const [layoutMode, setLayoutMode] = useState<DevLayoutMode>(() => {
    const raw = initialParams?.get("layout");
    if (raw === "stage" || raw === "split" || raw === "grid") return raw;
    return "split";
  });
  const [activeSlot, setActiveSlot] = useState<number>(() => {
    const raw = Number(initialParams?.get("slot") || "1");
    return Number.isFinite(raw) && raw >= 1 ? raw : 1;
  });
  const initialActionRef = useRef(initialParams?.get("action") || "seat_all");
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
        setActiveSlot((prev) =>
          json.players.some((p) => p.slot === prev) ? prev : (json.players[0]?.slot ?? 1),
        );
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
    const rawAct = initialActionRef.current;
    initialActionRef.current = "seat_all";
    void loadTokens(preset, rawAct === "none" ? "" : rawAct);
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
          <h1>
            <ExcaliburSealEmblem className="av-svg-md" />
            <span>阿瓦隆 · 卡美洛动态宽度联调台 (/dev)</span>
          </h1>
          <span style={{ fontSize: "11.5px", color: "var(--text-muted)" }}>
            支持全屏宽舞台 / 双栏分屏 / 多座网格切换 · 真实 <code>postMessage</code> 握手 + 非对称迷雾验证
          </span>
        </div>
        <div className="dev-toolbar-controls">
          <select
            className="av-select"
            aria-label="布局宽度模式"
            value={layoutMode}
            onChange={(e) => setLayoutMode(e.target.value as DevLayoutMode)}
          >
            <option value="stage">宽屏主舞台 (100% 全宽战情室)</option>
            <option value="split">双栏分屏圆桌 (≥560px 环形盘面)</option>
            <option value="grid">多座对比网格 (自适应多视角)</option>
          </select>

          {layoutMode === "stage" && (data?.players?.length ?? 0) > 0 && (
            <select
              className="av-select"
              aria-label="当前检视座位"
              value={activeSlot}
              onChange={(e) => setActiveSlot(Number(e.target.value))}
            >
              {(data?.players ?? []).map((p) => (
                <option key={p.slot} value={p.slot}>
                  视角：#{p.slot} {p.displayName}
                </option>
              ))}
            </select>
          )}

          <select
            className="av-select"
            aria-label="契约板子预设"
            value={preset}
            disabled={loading}
            onChange={(e) => setPreset(e.target.value)}
          >
            <option value="5p">5人标准局 (3蓝2红)</option>
            <option value="6p">6人标准局 (4蓝2红)</option>
            <option value="7p_oberon">7人奥伯伦局 (Q4保护轮)</option>
            <option value="7p_mordred">7人莫德雷德局 (Q4保护轮)</option>
            <option value="8p">8人圣战局 (5蓝3红)</option>
            <option value="10p">10人全神局 (6蓝4红)</option>
          </select>
          <button
            type="button"
            className="av-btn"
            disabled={loading}
            onClick={() => void loadTokens(preset, "seat_all")}
          >
            全员入座
          </button>
          <button
            type="button"
            className="av-btn av-btn-gold"
            disabled={loading}
            onClick={() => void loadTokens(preset, "start_game")}
          >
            ✦ 一键拔剑发牌
          </button>
          <button
            type="button"
            className="av-btn"
            disabled={loading}
            onClick={() => void loadTokens(preset, "reset_lobby")}
          >
            重置回大厅
          </button>
        </div>
      </header>

      {errMsg && <div className="av-toast-err">{errMsg}</div>}

      <div
        className={[
          "dev-frames-strip",
          layoutMode === "stage" ? "layout-stage" : "",
          layoutMode === "split" ? "layout-split" : "",
        ]
          .filter(Boolean)
          .join(" ")}
      >
        {(data?.players ?? []).map((p) => {
          const hiddenInStage = layoutMode === "stage" && p.slot !== activeSlot;
          return (
            <div
              key={p.slot}
              className="dev-frame-column"
              style={hiddenInStage ? { display: "none" } : undefined}
            >
              <div className="dev-frame-title">
                <span>骑士席位 #{p.slot}</span>
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
          );
        })}
      </div>
    </div>
  );
}
