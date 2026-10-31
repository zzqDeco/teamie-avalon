import type { ActionResponse, PlayerView } from "./types";

export class ApiUnauthorizedError extends Error {
  constructor(message = "Context Token expired or unauthorized") {
    super(message);
    this.name = "ApiUnauthorizedError";
  }
}

function makeIdempotencyKey(action: string): string {
  const rand =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
  return `web-${action}-${rand}`;
}

export async function fetchMyView(
  token: string,
  onUnauthorized?: () => void,
): Promise<PlayerView> {
  const resp = await fetch("/api/view", {
    method: "GET",
    headers: {
      Authorization: `Bearer ${token}`,
      Accept: "application/json",
    },
  });
  if (resp.status === 401) {
    onUnauthorized?.();
    throw new ApiUnauthorizedError();
  }
  if (!resp.ok) {
    const errBody = (await resp.json().catch(() => ({}))) as { message?: string };
    throw new Error(errBody.message || `获取视角失败 (HTTP ${resp.status})`);
  }
  return (await resp.json()) as PlayerView;
}

export async function postGameAction(
  token: string,
  action: string,
  body: Record<string, unknown> = {},
  onUnauthorized?: () => void,
): Promise<ActionResponse> {
  const resp = await fetch(`/api/action/${action}`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
      Accept: "application/json",
      "Idempotency-Key": makeIdempotencyKey(action),
    },
    body: JSON.stringify(body),
  });
  if (resp.status === 401) {
    onUnauthorized?.();
    throw new ApiUnauthorizedError();
  }
  const data = (await resp.json().catch(() => ({}))) as ActionResponse & {
    message?: string;
    error?: string;
  };
  if (!resp.ok) {
    throw new Error(data.message || data.error || `操作失败 (HTTP ${resp.status})`);
  }
  return data;
}

/**
 * Connects to GET /api/stream using fetch + ReadableStream so the Bearer Context Token
 * stays strictly in the Authorization header (never exposed in URL query strings).
 */
export function subscribeViewStream(opts: {
  token: string;
  onView: (view: PlayerView) => void;
  onStatus: (connected: boolean) => void;
  onUnauthorized: () => void;
}): () => void {
  const controller = new AbortController();
  let stopped = false;

  const run = async () => {
    let backoffMs = 800;
    while (!stopped && !controller.signal.aborted) {
      try {
        const resp = await fetch("/api/stream", {
          method: "GET",
          headers: {
            Authorization: `Bearer ${opts.token}`,
            Accept: "text/event-stream",
          },
          signal: controller.signal,
        });
        if (resp.status === 401) {
          opts.onStatus(false);
          opts.onUnauthorized();
          return;
        }
        if (!resp.ok || !resp.body) {
          throw new Error(`SSE HTTP ${resp.status}`);
        }
        opts.onStatus(true);
        backoffMs = 800;

        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";

        while (!stopped && !controller.signal.aborted) {
          const { value, done } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });

          let sepIdx: number;
          while ((sepIdx = buffer.indexOf("\n\n")) !== -1) {
            const rawBlock = buffer.slice(0, sepIdx);
            buffer = buffer.slice(sepIdx + 2);
            const lines = rawBlock.split(/\r?\n/);
            const dataLines: string[] = [];
            for (const line of lines) {
              if (line.startsWith("data:")) {
                dataLines.push(line.slice(5).trim());
              }
            }
            if (dataLines.length > 0) {
              try {
                const parsed = JSON.parse(dataLines.join("\n")) as PlayerView;
                opts.onView(parsed);
              } catch {
                // Ignore malformed keepalive chunks
              }
            }
          }
        }
      } catch {
        if (stopped || controller.signal.aborted) return;
        opts.onStatus(false);
        await new Promise((r) => setTimeout(r, backoffMs));
        backoffMs = Math.min(backoffMs * 1.8, 8000);
      }
    }
  };

  void run();
  return () => {
    stopped = true;
    controller.abort();
  };
}
