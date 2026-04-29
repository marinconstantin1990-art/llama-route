<script lang="ts">
  import { onMount } from "svelte";
  import {
    fetchGPUs,
    rescanGPUs,
    setGPUEnabled,
    fetchModels,
    saveModel,
    deleteModel,
    CMD_PLACEHOLDER,
    type GPU,
    type ModelSettings,
  } from "../stores/settings";

  let gpus: GPU[] = $state([]);
  let models: ModelSettings[] = $state([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Editor dialog state
  let editorOpen = $state(false);
  let editorIsUpdate = $state(false);
  let formId = $state("");
  let formName = $state("");
  let formCmd = $state("");
  let formConcurrency = $state(8);
  let formAutoScale = $state(false);
  let formMaxInstances = $state(2);
  let formAllowedGPUs: string[] = $state([]);

  async function refresh() {
    loading = true;
    error = null;
    try {
      [gpus, models] = await Promise.all([fetchGPUs(), fetchModels()]);
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loading = false;
    }
  }

  async function onToggleGPU(g: GPU, enabled: boolean) {
    try {
      await setGPUEnabled(g.id, enabled);
      g.enabled = enabled;
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function onRescan() {
    try {
      gpus = await rescanGPUs();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  function openCreate() {
    editorIsUpdate = false;
    formId = "";
    formName = "";
    formCmd = "";
    formConcurrency = 8;
    formAutoScale = false;
    formMaxInstances = 2;
    formAllowedGPUs = gpus.filter((g) => g.enabled).map((g) => g.id);
    editorOpen = true;
  }

  function openEdit(m: ModelSettings) {
    editorIsUpdate = true;
    formId = m.id;
    formName = m.name ?? "";
    formCmd = m.cmd ?? "";
    formConcurrency = m.concurrencyLimit || 8;
    formAutoScale = m.autoScale?.enabled ?? false;
    formMaxInstances = m.autoScale?.maxInstances || 2;
    formAllowedGPUs = m.autoScale?.allowedGPUs ?? gpus.filter((g) => g.enabled).map((g) => g.id);
    editorOpen = true;
  }

  async function onSave() {
    error = null;
    if (!formId.trim()) {
      error = "Model id is required";
      return;
    }
    const cmd = formCmd.trim();
    if (!cmd) {
      error = "Command is required";
      return;
    }
    if (!cmd.includes("${PORT}")) {
      error = "Command must reference \${PORT} so the proxy can assign a port";
      return;
    }
    const payload: ModelSettings = {
      id: formId.trim(),
      name: formName,
      cmd,
      concurrencyLimit: formConcurrency,
      autoScale: {
        enabled: formAutoScale,
        maxInstances: formMaxInstances,
        allowedGPUs: formAllowedGPUs,
      },
    };
    try {
      await saveModel(payload, editorIsUpdate);
      editorOpen = false;
      await refresh();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function onDelete(m: ModelSettings) {
    if (!confirm(`Delete model "${m.id}"?`)) return;
    try {
      await deleteModel(m.id);
      await refresh();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  function toggleAllowedGPU(id: string) {
    if (formAllowedGPUs.includes(id)) {
      formAllowedGPUs = formAllowedGPUs.filter((g) => g !== id);
    } else {
      formAllowedGPUs = [...formAllowedGPUs, id];
    }
  }

  onMount(refresh);
</script>

<div class="space-y-6">
  {#if error}
    <div class="rounded border border-red-400 bg-red-50 p-2 text-sm text-red-700 dark:bg-red-900 dark:text-red-200">
      {error}
    </div>
  {/if}

  <section>
    <div class="flex items-center justify-between mb-2">
      <h2 class="text-lg font-semibold">GPUs</h2>
      <button class="px-3 py-1 rounded border border-border" onclick={onRescan}>Rescan</button>
    </div>
    {#if loading}
      <p class="text-gray-500">Loading…</p>
    {:else if gpus.length === 0}
      <p class="text-gray-500">No GPUs detected. Install <code>nvidia-smi</code> / <code>rocm-smi</code> / <code>xpu-smi</code> as appropriate.</p>
    {:else}
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left border-b border-border">
            <th class="py-1">Enabled</th>
            <th>ID</th>
            <th>Vendor</th>
            <th>Name</th>
            <th>VRAM total</th>
            <th>VRAM free</th>
          </tr>
        </thead>
        <tbody>
          {#each gpus as g (g.id)}
            <tr class="border-b border-border/60">
              <td class="py-1">
                <input type="checkbox" checked={g.enabled} onchange={(e) => onToggleGPU(g, (e.currentTarget as HTMLInputElement).checked)} />
              </td>
              <td><code>{g.id}</code></td>
              <td>{g.vendor}</td>
              <td>{g.name}</td>
              <td>{g.vramTotalMB} MB</td>
              <td>{g.vramFreeMB} MB</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>

  <section>
    <div class="flex items-center justify-between mb-2">
      <h2 class="text-lg font-semibold">Models</h2>
      <button class="px-3 py-1 rounded border border-border" onclick={openCreate}>Add model</button>
    </div>
    {#if loading}
      <p class="text-gray-500">Loading…</p>
    {:else if models.length === 0}
      <p class="text-gray-500">No models configured yet.</p>
    {:else}
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left border-b border-border">
            <th class="py-1">ID</th>
            <th>Cmd</th>
            <th>Slots</th>
            <th>Auto-scale</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each models as m (m.id)}
            <tr class="border-b border-border/60 align-top">
              <td class="py-1 font-medium"><code>{m.id}</code></td>
              <td class="break-all"><code class="text-xs">{m.cmd}</code></td>
              <td>{m.concurrencyLimit || "—"}</td>
              <td>{m.autoScale?.enabled ? `up to ${m.autoScale.maxInstances}` : "off"}</td>
              <td class="whitespace-nowrap">
                <button class="px-2 py-0.5 rounded border border-border mr-1" onclick={() => openEdit(m)}>Edit</button>
                <button class="px-2 py-0.5 rounded border border-red-400 text-red-700" onclick={() => onDelete(m)}>Delete</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>

  {#if editorOpen}
    <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onclick={() => (editorOpen = false)} onkeydown={(e) => e.key === "Escape" && (editorOpen = false)} role="dialog" tabindex="-1">
      <div class="bg-surface text-foreground p-4 rounded shadow-lg w-[640px] max-w-full max-h-[90vh] overflow-auto" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="document">
        <h3 class="text-lg font-semibold mb-3">{editorIsUpdate ? "Edit model" : "Add model"}</h3>

        <label class="block mb-2">
          <span class="text-sm">Model ID</span>
          <input class="w-full border border-border rounded px-2 py-1 bg-transparent" disabled={editorIsUpdate} bind:value={formId} placeholder="qwen-coder" />
        </label>

        <label class="block mb-2">
          <span class="text-sm">Display name (optional)</span>
          <input class="w-full border border-border rounded px-2 py-1 bg-transparent" bind:value={formName} />
        </label>

        <label class="block mb-2">
          <span class="text-sm">Command</span>
          <textarea
            class="w-full border border-border rounded px-2 py-1 bg-transparent font-mono text-xs"
            rows="8"
            bind:value={formCmd}
            placeholder={CMD_PLACEHOLDER}
          ></textarea>
          <span class="text-xs text-gray-500">
            Free-form. Anything that listens on the substituted <code>${"${PORT}"}</code> works —
            llama-server, vLLM, tabbyAPI, sd-server, a docker run, etc.
          </span>
        </label>

        <label class="block mb-2">
          <span class="text-sm">Concurrency limit (slots per instance)</span>
          <input type="number" min="1" class="w-32 border border-border rounded px-2 py-1 bg-transparent" bind:value={formConcurrency} />
        </label>

        <label class="block mb-2">
          <input type="checkbox" bind:checked={formAutoScale} />
          <span class="text-sm">Auto-scale across GPUs when slots fill</span>
        </label>

        {#if formAutoScale}
          <label class="block mb-2 ml-5">
            <span class="text-sm">Max instances</span>
            <input type="number" min="1" class="w-32 border border-border rounded px-2 py-1 bg-transparent" bind:value={formMaxInstances} />
          </label>
          <div class="ml-5 mb-2">
            <span class="text-sm">Allowed GPUs</span>
            <div class="text-xs">
              {#each gpus as g (g.id)}
                <label class="block">
                  <input type="checkbox" checked={formAllowedGPUs.includes(g.id)} onchange={() => toggleAllowedGPU(g.id)} />
                  <code>{g.id}</code> — {g.name}
                </label>
              {/each}
            </div>
          </div>
        {/if}

        <div class="flex justify-end gap-2 mt-4">
          <button class="px-3 py-1 rounded border border-border" onclick={() => (editorOpen = false)}>Cancel</button>
          <button class="px-3 py-1 rounded bg-blue-600 text-white" onclick={onSave}>Save</button>
        </div>
      </div>
    </div>
  {/if}
</div>
