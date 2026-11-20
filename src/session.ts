// One Things account: its cursor, task graph and the fail-closed sync/write
// lifecycle. The Durable Object in index.ts owns one Session and serializes
// every operation on it; persistence goes through the Store interface so the
// same code runs in tests without a Durable Object.

import { State, ThingsClient, Cursor, RawItem, CommitUncertainError, APIError, EntityKind, API_ENDPOINT } from "./things";
import { runTool, Envelope, ToolError, activeCounts } from "./tools";
import { diagnose } from "./diagnose";

export interface Meta {
  cred: string; // sha256(lower(email) \0 password) of the last verified credential
  cursor: Cursor;
}

export interface Store {
  loadMeta(): Meta | null;
  loadState(): State;
  // Persists the cursor plus the given entities (absent from state = deleted).
  save(meta: Meta, state: State, changed: { kind: EntityKind; uuid: string }[]): void;
  replaceAll(meta: Meta, state: State): void;
}

const SYNC_DEBOUNCE_MS = 2000;
const OUTDATED_ANCESTOR_RETRIES = 3;

export async function credentialDigest(email: string, password: string): Promise<string> {
  const data = new TextEncoder().encode(email.trim().toLowerCase() + "\x00" + password);
  return [...new Uint8Array(await crypto.subtle.digest("SHA-256", data))].map((b) => b.toString(16).padStart(2, "0")).join("");
}

export class Session {
  private meta: Meta | null = null;
  private state: State | null = null;
  private client: ThingsClient | null = null;
  private lastSyncAt = 0;
  private lastWriteAt = 0;
  private chain: Promise<unknown> = Promise.resolve();

  constructor(private store: Store, private endpoint = API_ENDPOINT) {}

  // Every operation runs alone: cursor and graph advance atomically.
  private serialize<T>(fn: () => Promise<T>): Promise<T> {
    const run = this.chain.then(fn, fn);
    this.chain = run.catch(() => {});
    return run;
  }

  callTool(email: string, password: string, name: string, args: Record<string, any>): Promise<{ data?: any; error?: string }> {
    return this.serialize(async () => {
      try {
        await this.open(email, password);
      } catch (e: any) {
        return { error: e instanceof ToolError ? e.message : `login: ${e?.message ?? e}` };
      }
      try {
        await this.syncIfStale();
      } catch (e: any) {
        return { error: `sync: ${e?.message ?? e}` };
      }
      try {
        const data = await runTool({ state: this.state!, write: (envs) => this.write(envs) }, name, args);
        return { data };
      } catch (e: any) {
        if (e instanceof ToolError) return { error: e.message };
        throw e;
      }
    });
  }

  runDiagnose(email: string, password: string): Promise<{ data?: any; error?: string }> {
    return this.serialize(async () => {
      try {
        await this.open(email, password);
      } catch (e: any) {
        return { error: `login: ${e?.message ?? e}` };
      }
      const report = await diagnose(email.trim(), password, this.endpoint, async () => {
        await this.syncIfStale();
        return activeCounts(this.state!);
      }, () => ({ areas: this.state!.areas.size, tags: this.state!.tags.size }));
      return { data: report };
    });
  }

  // Binds the session to a verified credential. A credential that differs
  // from the last verified one is checked against Things Cloud first, so a
  // wrong password never reads the cached graph. The graph is rebuilt from
  // scratch only when there is none yet or the account's history changed.
  private async open(email: string, password: string): Promise<void> {
    email = email.trim();
    const digest = await credentialDigest(email, password);
    if (!this.meta) this.meta = this.store.loadMeta();
    if (this.meta && this.meta.cred === digest) {
      if (!this.client) this.client = new ThingsClient(email, password, this.endpoint);
      if (!this.state) this.state = this.store.loadState();
      return;
    }
    const client = new ThingsClient(email, password, this.endpoint);
    const verified = await client.verify();
    const cursor = await client.history(verified.historyKey).catch((e) => {
      throw new Error(`get authoritative history ${verified.historyKey}: ${e?.message ?? e}`);
    });
    this.client = client;
    if (this.meta && this.meta.cursor.id === verified.historyKey) {
      if (!this.state) this.state = this.store.loadState();
      this.meta = { ...this.meta, cred: digest };
      this.store.save(this.meta, this.state, []);
      return;
    }
    await this.fullRebuild(cursor.id, digest);
  }

  // Initial load only; never used as a fallback after an incremental error.
  private async fullRebuild(historyID: string, cred: string): Promise<void> {
    const cur: Cursor = { id: historyID, latestServerIndex: 0, loadedServerIndex: 0, latestSchemaVersion: 0 };
    const all: RawItem[] = [];
    let start = 0;
    for (;;) {
      const { items, hasMore } = await this.client!.items(cur, start).catch((e) => {
        throw new Error(`fetch items: ${e?.message ?? e}`);
      });
      if (items.length === 0) break;
      all.push(...items);
      if (!hasMore) break;
      start = cur.loadedServerIndex;
    }
    const state = new State();
    try {
      state.update(all);
    } catch (e: any) {
      throw new Error(`rebuild state: ${e?.message ?? e}`);
    }
    state.takeDirty();
    this.state = state;
    this.meta = { cred, cursor: cur };
    this.store.replaceAll(this.meta, state);
    this.lastSyncAt = Date.now();
  }

  private async syncIfStale(): Promise<void> {
    const now = Date.now();
    if (now - this.lastSyncAt < SYNC_DEBOUNCE_MS && !(this.lastWriteAt > this.lastSyncAt)) return;
    await this.syncOrRecover();
    this.lastSyncAt = Date.now();
  }

  // Incremental sync, except when the account's history was replaced (a Things
  // Cloud account reset): Things Cloud then answers 404 AccountIssue for the old
  // cursor, or a current index below it. Only if Verify confirms a different history key, or a head below our
  // cursor, is the graph rebuilt from scratch; any other error stays an error.
  private async syncOrRecover(): Promise<void> {
    try {
      await this.incrementalSync();
    } catch (e) {
      const cause = (e as any)?.cause;
      const replaced = (cause instanceof APIError && cause.status === 404) || /cursor regressed/.test(String(cause?.message));
      if (!replaced) throw e;
      const verified = await this.client!.verify();
      const head = await this.client!.history(verified.historyKey);
      const cur = this.meta!.cursor;
      if (verified.historyKey === cur.id && head.latestServerIndex >= cur.loadedServerIndex) throw e;
      await this.fullRebuild(verified.historyKey, this.meta!.cred);
    }
  }

  // Fetches into a cloned cursor; the live cursor and graph advance only after
  // every page decoded and the whole delta applied.
  private async incrementalSync(): Promise<RawItem[]> {
    const cur: Cursor = { ...this.meta!.cursor };
    const delta: RawItem[] = [];
    let start = cur.loadedServerIndex;
    for (;;) {
      let page;
      try {
        page = await this.client!.items(cur, start);
      } catch (e: any) {
        throw new Error(`incremental fetch at index ${start}: ${e?.message ?? e}`, { cause: e });
      }
      if (page.items.length === 0) break;
      delta.push(...page.items);
      if (!page.hasMore) break;
      start = cur.loadedServerIndex;
    }
    if (delta.length > 0) {
      try {
        this.state!.update(delta);
      } catch (e: any) {
        throw new Error(`apply incremental state: ${e?.message ?? e}`);
      }
    }
    this.meta = { ...this.meta!, cursor: cur };
    this.store.save(this.meta, this.state!, this.state!.takeDirty());
    return delta;
  }

  private async write(envs: Envelope[]): Promise<void> {
    // The exact events Things Cloud will record, after a JSON round trip.
    const expected: RawItem[] = envs.map((e) => ({ uuid: e.uuid, t: e.t, e: e.e, p: JSON.parse(JSON.stringify(e.p)) }));
    for (const e of expected) if (!e.uuid) throw new Error("validate commit: item UUID is required");
    try {
      new State().update(expected);
    } catch (e: any) {
      throw new Error(`validate commit payload: ${e?.message ?? e}`);
    }
    for (let attempt = 0; ; attempt++) {
      try {
        await this.syncOrRecover();
      } catch (e: any) {
        throw new Error(`pre-write sync: ${e?.message ?? e}`);
      }
      const cur: Cursor = { ...this.meta!.cursor };
      try {
        await this.client!.commit(cur, expected);
      } catch (e: any) {
        // 409 OutdatedAncestor: another device committed after our sync. The
        // commit was rejected cleanly, so sync again and resend.
        if (e instanceof APIError && e.status === 409 && attempt < OUTDATED_ANCESTOR_RETRIES) continue;
        if (e instanceof CommitUncertainError) {
          // Never retry: read the authoritative history to see whether it landed.
          let delta: RawItem[] = [];
          try {
            delta = await this.incrementalSync();
          } catch {}
          if (containsExpected(delta, expected)) return;
          throw new Error(`${e.message}; reconciliation did not confirm the commit, do not retry automatically`);
        }
        throw new Error(`commit rejected: ${e?.message ?? e}`);
      }
      // A confirmed commit advances exactly one index: apply the same events
      // locally so a later read failure cannot turn success into failure.
      try {
        this.state!.update(expected);
      } catch (e: any) {
        throw new Error(`commit succeeded but local state apply failed: ${e?.message ?? e}`);
      }
      cur.loadedServerIndex = cur.latestServerIndex;
      this.meta = { ...this.meta!, cursor: cur };
      this.store.save(this.meta, this.state!, this.state!.takeDirty());
      this.lastWriteAt = this.lastSyncAt = Date.now();
      return;
    }
  }
}

function containsExpected(delta: RawItem[], expected: RawItem[]): boolean {
  const seen = new Set(delta.map((d) => `${d.uuid}|${d.e}:${d.t}`));
  return expected.every((e) => seen.has(`${e.uuid}|${e.e}:${e.t}`));
}
