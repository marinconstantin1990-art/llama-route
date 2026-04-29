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

// CMD_PLACEHOLDER is the example shown in the Settings form. It demonstrates
// the only contract the proxy enforces: somewhere in the command, use
// ${PORT} so the auto-port allocator can substitute a free port at launch.
export const CMD_PLACEHOLDER = `llama-server --port \${PORT} \\
  --ctx-size 65536 -fa on -b 2048 -ub 2048 -ngl -1 \\
  --temperature 0 \\
  -hf Jackrong/Qwopus3.5-9B-v3-GGUF:Q8_0

# Other backends work too — the proxy just runs whatever you put here:
#   python -m vllm.entrypoints.openai.api_server --port \${PORT} --model Qwen/Qwen2.5-Coder-7B-Instruct
#   sd-server --port \${PORT} --model /models/sdxl.gguf
#   docker run --rm -p \${PORT}:8000 ghcr.io/your/server:tag`;
