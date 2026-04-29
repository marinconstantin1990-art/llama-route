// Client for /api/gpus and /api/config/* endpoints used by the Settings page.

export interface GPU {
  id: string;
  vendor: string;
  index: number;
  name: string;
  vramTotalMB: number;
  vramFreeMB: number;
  enabled: boolean;
}

export interface AutoScale {
  enabled: boolean;
  maxInstances: number;
  allowedGPUs: string[];
}

export interface ModelSettings {
  id: string;
  name?: string;
  description?: string;
  cmd: string;
  concurrencyLimit: number;
  autoScale: AutoScale;
  aliases?: string[];
}

async function jsonOrThrow<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${text}`);
  }
  return (await res.json()) as T;
}

export async function fetchGPUs(): Promise<GPU[]> {
  const res = await fetch("/api/gpus");
  const body = await jsonOrThrow<{ gpus: GPU[] }>(res);
  return body.gpus ?? [];
}

export async function rescanGPUs(): Promise<GPU[]> {
  const res = await fetch("/api/gpus/rescan", { method: "POST" });
  const body = await jsonOrThrow<{ gpus: GPU[] }>(res);
  return body.gpus ?? [];
}

export async function setGPUEnabled(id: string, enabled: boolean): Promise<void> {
  const res = await fetch(`/api/gpus/${encodeURIComponent(id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  });
  await jsonOrThrow<unknown>(res);
}

export async function fetchModels(): Promise<ModelSettings[]> {
  const res = await fetch("/api/config/models");
  const body = await jsonOrThrow<{ models: ModelSettings[] }>(res);
  return body.models ?? [];
}

export async function saveModel(m: ModelSettings, isUpdate: boolean): Promise<void> {
  const url = isUpdate ? `/api/config/models/${encodeURIComponent(m.id)}` : "/api/config/models";
  const res = await fetch(url, {
    method: isUpdate ? "PUT" : "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(m),
  });
  await jsonOrThrow<unknown>(res);
}

export async function deleteModel(id: string): Promise<void> {
  const res = await fetch(`/api/config/models/${encodeURIComponent(id)}`, { method: "DELETE" });
  await jsonOrThrow<unknown>(res);
}

// composeCmd builds a llama-server command line from the form fields. The
// resulting string is what gets stored under models.<id>.cmd in config.yaml.
export function composeCmd(opts: {
  source: "local" | "hf";
  modelPath: string;
  hfRepo: string;
  extraParams: string;
  port?: string; // defaults to ${PORT} for the proxy auto-port allocator
}): string {
  const port = opts.port ?? "${PORT}";
  const parts: string[] = ["llama-server"];
  const extra = opts.extraParams.trim();
  if (extra) parts.push(extra);
  if (opts.source === "hf" && opts.hfRepo.trim()) {
    parts.push(`-hf ${opts.hfRepo.trim()}`);
  } else if (opts.source === "local" && opts.modelPath.trim()) {
    parts.push(`-m ${opts.modelPath.trim()}`);
  }
  parts.push(`--port ${port}`);
  return parts.join(" ");
}
