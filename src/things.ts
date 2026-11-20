// Things Cloud protocol: identifiers, HTTP client, and the task graph built
// from history items. Ported from the Go things-cloud-sdk this service used to
// run on; the validation rules are the same (fail closed on anything unknown).

export const API_ENDPOINT = "https://cloud.culturedcode.com";
export const SUPPORTED_SCHEMA = 301;
const USER_AGENT = "ThingsMac/32209501";
const CLIENT_INFO = btoa(
  JSON.stringify({
    dm: "MacBookPro18,3",
    lr: "US",
    nf: true,
    nk: true,
    nn: "ThingsMac",
    nv: "32209501",
    on: "macOS",
    ov: "15.7.3",
    pl: "en-US",
    ul: "en-Latn-US",
  }),
);

// ---------------------------------------------------------------------------
// Identifiers: Base58 of a 16-byte UUID
// ---------------------------------------------------------------------------

const B58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz";

export function encodeUUIDBytes(u: Uint8Array): string {
  let zeros = 0;
  while (zeros < u.length && u[zeros] === 0) zeros++;
  let n = 0n;
  for (const b of u) n = (n << 8n) | BigInt(b);
  let out = "";
  while (n > 0n) {
    out = B58[Number(n % 58n)] + out;
    n /= 58n;
  }
  return "1".repeat(zeros) + out;
}

export function validUUID(s: string): boolean {
  if (typeof s !== "string" || s === "") return false;
  let zeros = 0;
  while (zeros < s.length && s[zeros] === "1") zeros++;
  let n = 0n;
  for (let i = zeros; i < s.length; i++) {
    const idx = B58.indexOf(s[i]);
    if (idx < 0) return false;
    n = n * 58n + BigInt(idx);
  }
  let len = 0;
  while (n > 0n) {
    len++;
    n >>= 8n;
  }
  return zeros + len === 16;
}

export function newUUID(): string {
  const b = new Uint8Array(16);
  crypto.getRandomValues(b);
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  return encodeUUIDBytes(b);
}

// SHA-1 of the legacy identifier (with the recurrence-suffix rule), first 16
// bytes, Base58. Synchronous SHA-1 because state updates are synchronous.
export function encodeLegacyIdentifier(id: string): string {
  if (id === "") return "";
  const enc = new TextEncoder();
  let input = enc.encode(id);
  const m = /^([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})(-\d{8})$/.exec(id);
  if (m) {
    const prefixSum = sha1(enc.encode(m[1])).slice(0, 16);
    const suffix = enc.encode(m[2]);
    input = new Uint8Array(16 + suffix.length);
    input.set(prefixSum, 0);
    input.set(suffix, 16);
  }
  return encodeUUIDBytes(sha1(input).slice(0, 16));
}

function sha1(msg: Uint8Array): Uint8Array {
  const ml = msg.length;
  const withPad = new Uint8Array(((ml + 9 + 63) >> 6) << 6);
  withPad.set(msg);
  withPad[ml] = 0x80;
  const dv = new DataView(withPad.buffer);
  dv.setUint32(withPad.length - 4, (ml * 8) >>> 0);
  dv.setUint32(withPad.length - 8, Math.floor((ml * 8) / 2 ** 32));
  let h0 = 0x67452301, h1 = 0xefcdab89, h2 = 0x98badcfe, h3 = 0x10325476, h4 = 0xc3d2e1f0;
  const w = new Uint32Array(80);
  for (let off = 0; off < withPad.length; off += 64) {
    for (let i = 0; i < 16; i++) w[i] = dv.getUint32(off + i * 4);
    for (let i = 16; i < 80; i++) {
      const x = w[i - 3] ^ w[i - 8] ^ w[i - 14] ^ w[i - 16];
      w[i] = (x << 1) | (x >>> 31);
    }
    let a = h0, b = h1, c = h2, d = h3, e = h4;
    for (let i = 0; i < 80; i++) {
      let f: number, k: number;
      if (i < 20) { f = (b & c) | (~b & d); k = 0x5a827999; }
      else if (i < 40) { f = b ^ c ^ d; k = 0x6ed9eba1; }
      else if (i < 60) { f = (b & c) | (b & d) | (c & d); k = 0x8f1bbcdc; }
      else { f = b ^ c ^ d; k = 0xca62c1d6; }
      const t = (((a << 5) | (a >>> 27)) + f + e + k + w[i]) >>> 0;
      e = d; d = c; c = (b << 30) | (b >>> 2); b = a; a = t;
    }
    h0 = (h0 + a) >>> 0; h1 = (h1 + b) >>> 0; h2 = (h2 + c) >>> 0; h3 = (h3 + d) >>> 0; h4 = (h4 + e) >>> 0;
  }
  const out = new Uint8Array(20);
  const odv = new DataView(out.buffer);
  [h0, h1, h2, h3, h4].forEach((h, i) => odv.setUint32(i * 4, h));
  return out;
}

// ---------------------------------------------------------------------------
// HTTP client
// ---------------------------------------------------------------------------

export class ThingsError extends Error {}

export class APIError extends ThingsError {
  constructor(public status: number, statusText: string, public thingsResponse: string) {
    super(thingsResponse ? `things cloud: ${status} ${statusText} (things-response: ${thingsResponse})` : `things cloud: ${status} ${statusText}`);
  }
}

export class UnauthorizedError extends ThingsError {
  constructor() {
    super("unauthorized");
  }
}

export class CommitUncertainError extends ThingsError {
  constructor(cause: string) {
    super(`commit outcome is uncertain: ${cause}`);
  }
}

export interface RawItem {
  uuid: string;
  p: any;
  e: string;
  t: number;
}

export interface Cursor {
  id: string;
  latestServerIndex: number;
  loadedServerIndex: number;
  latestSchemaVersion: number;
}

// One client per account. Requests are spaced at least a second apart, as the
// Go SDK's rate limiter did, so a burst of tool calls cannot trip Things
// Cloud's abuse prevention.
export class ThingsClient {
  private nextAt = 0;
  constructor(public email: string, private password: string, public endpoint = API_ENDPOINT) {}

  private async request(method: string, path: string, opts: { query?: Record<string, string>; headers?: Record<string, string>; body?: string } = {}): Promise<Response> {
    const wait = this.nextAt - Date.now();
    if (wait > 0) await new Promise((r) => setTimeout(r, wait));
    this.nextAt = Date.now() + 1000;
    const url = new URL(this.endpoint + path);
    for (const [k, v] of Object.entries(opts.query ?? {})) url.searchParams.set(k, v);
    const headers: Record<string, string> = {
      "User-Agent": USER_AGENT,
      Accept: "application/json",
      "Accept-Charset": "UTF-8",
      "Accept-Language": "en-US,en;q=0.9",
      "Things-Client-Info": CLIENT_INFO,
      ...opts.headers,
    };
    if (method !== "GET" && method !== "HEAD" && method !== "DELETE") {
      headers["Content-Type"] = "application/json; charset=UTF-8";
    }
    // Things Cloud answers bursts with 429 AbusePrevention. Reads are safe to
    // repeat, so they back off and retry a few times; a commit never retries.
    for (let attempt = 0; ; attempt++) {
      const resp = await fetch(url.toString(), { method, headers, body: opts.body, signal: AbortSignal.timeout(30000) });
      if (resp.status !== 429 || method !== "GET" || attempt >= 3) return resp;
      const after = Number(resp.headers.get("Retry-After"));
      const delay = Number.isFinite(after) && after > 0 ? Math.min(after, 10) * 1000 : 2000 * 2 ** attempt;
      await resp.body?.cancel();
      await new Promise((r) => setTimeout(r, delay));
      this.nextAt = Date.now() + 1000;
    }
  }

  private accountPath(): string {
    return `/version/1/account/${encodeURI(this.email)}`;
  }

  async verify(): Promise<{ historyKey: string; email: string; status: string }> {
    const resp = await this.request("GET", this.accountPath(), { headers: { Authorization: `Password ${this.password}` } });
    if (resp.status !== 200) {
      if (resp.status === 401) throw new UnauthorizedError();
      throw new APIError(resp.status, resp.statusText, resp.headers.get("Things-Response") ?? "");
    }
    let v: any = {};
    try {
      v = await resp.json();
    } catch {}
    return { historyKey: String(v?.["history-key"] ?? ""), email: String(v?.email ?? ""), status: String(v?.status ?? "") };
  }

  async history(id: string): Promise<Cursor> {
    const resp = await this.request("GET", `/version/1/history/${id}`);
    if (resp.status !== 200) {
      if (resp.status === 401) throw new UnauthorizedError();
      throw new APIError(resp.status, resp.statusText, resp.headers.get("Things-Response") ?? "");
    }
    const h: any = await resp.json();
    return { id, latestServerIndex: intOr0(h["latest-server-index"]), loadedServerIndex: 0, latestSchemaVersion: intOr0(h["latest-schema-version"]) };
  }

  async ownHistoryKeys(): Promise<string[]> {
    const resp = await this.request("GET", `${this.accountPath()}/own-history-keys`);
    if (resp.status !== 200) {
      if (resp.status === 401) throw new UnauthorizedError();
      throw new APIError(resp.status, resp.statusText, resp.headers.get("Things-Response") ?? "");
    }
    const keys: any = await resp.json().catch(() => []);
    return Array.isArray(keys) ? keys.map(String) : [];
  }

  // Fetches one page starting at startIndex and advances the cursor in place.
  async items(cur: Cursor, startIndex: number): Promise<{ items: RawItem[]; hasMore: boolean }> {
    if (startIndex < 0) throw new ThingsError("start index must be non-negative");
    const resp = await this.request("GET", `/version/1/history/${cur.id}/items`, { query: { "start-index": String(startIndex) } });
    if (resp.status !== 200) throw new APIError(resp.status, resp.statusText, resp.headers.get("Things-Response") ?? "");
    const v: any = await resp.json();
    if (typeof v !== "object" || v === null) throw new ThingsError("decode items response");
    const rawItems = v.items ?? [];
    if (!Array.isArray(rawItems)) throw new ThingsError("decode items response: items is not an array");
    const schema = intOr0(v.schema) || intOr0(v["latest-schema-version"]);
    if (schema > SUPPORTED_SCHEMA) throw new ThingsError(`unsupported Things Cloud schema ${schema} (maximum supported ${SUPPORTED_SCHEMA})`);
    const current = intOr0(v["current-item-index"]);
    if (current < startIndex) throw new ThingsError(`server cursor regressed from requested index ${startIndex} to ${current}`);
    const next = startIndex + rawItems.length;
    if (rawItems.length === 0 && startIndex < current) throw new ThingsError(`server returned no progress at index ${startIndex} while current index is ${current}`);
    if (schema === SUPPORTED_SCHEMA && next > current) throw new ThingsError(`server page ends at index ${next} beyond current index ${current}`);
    const items: RawItem[] = [];
    for (const m of rawItems) {
      if (typeof m !== "object" || m === null || Array.isArray(m)) throw new ThingsError("decode items response: malformed item map");
      for (const [uuid, it] of Object.entries<any>(m)) {
        if (typeof it !== "object" || it === null) throw new ThingsError(`decode item ${uuid}: not an object`);
        const e = it.e ?? "";
        const t = it.t ?? 0;
        if (typeof e !== "string" || !Number.isInteger(t)) throw new ThingsError(`decode item ${uuid}: malformed envelope`);
        items.push({ uuid, p: it.p === undefined ? null : it.p, e, t });
      }
    }
    cur.loadedServerIndex = next;
    cur.latestServerIndex = current;
    cur.latestSchemaVersion = schema;
    return { items, hasMore: rawItems.length > 0 && cur.loadedServerIndex < cur.latestServerIndex };
  }

  async commit(cur: Cursor, items: { uuid: string; t: number; e: string; p: any }[]): Promise<void> {
    if (items.length === 0) throw new ThingsError("commit contains no items");
    if (cur.latestSchemaVersion > SUPPORTED_SCHEMA) throw new ThingsError(`refusing write for unsupported Things Cloud schema ${cur.latestSchemaVersion} (maximum supported ${SUPPORTED_SCHEMA})`);
    const body: Record<string, { t: number; e: string; p: any }> = {};
    for (const it of items) {
      if (!it.uuid) throw new ThingsError("commit contains an item with an empty UUID");
      if (body[it.uuid]) throw new ThingsError(`commit contains duplicate UUID "${it.uuid}"`);
      body[it.uuid] = { t: it.t, e: it.e, p: it.p };
    }
    let resp: Response;
    try {
      resp = await this.request("POST", `/version/1/history/${cur.id}/commit`, {
        query: { "ancestor-index": String(cur.latestServerIndex), _cnt: "1" },
        headers: {
          Schema: "301",
          "Push-Priority": "5",
          "App-Instance-Id": "000000000000000000000000000000000000000000000000000000000000000-com.culturedcode.ThingsMac-000000000000000000000000000000000000000000000000000000000000000",
          "App-Id": "com.culturedcode.ThingsMac",
        },
        body: JSON.stringify(body),
      });
    } catch (e) {
      throw new CommitUncertainError(String(e));
    }
    if (resp.status !== 200) throw new APIError(resp.status, resp.statusText, resp.headers.get("Things-Response") ?? "");
    let w: any;
    try {
      w = await resp.json();
    } catch (e) {
      throw new CommitUncertainError(`decode commit response: ${e}`);
    }
    const head = w?.["server-head-index"];
    if (head !== cur.latestServerIndex + 1) {
      throw new CommitUncertainError(`unexpected server head index ${head} after ancestor ${cur.latestServerIndex}`);
    }
    cur.latestServerIndex = head;
  }
}

function intOr0(v: any): number {
  return Number.isInteger(v) ? v : 0;
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

// Dates are Unix seconds (fractional). creationDate null is Go's zero time.
export interface Task {
  uuid: string;
  creationDate: number | null;
  modificationDate: number | null;
  status: number;
  title: string;
  note: string;
  scheduledDate: number | null;
  completionDate: number | null;
  deadlineDate: number | null;
  index: number;
  areaIDs: string[] | null;
  parentTaskIDs: string[] | null;
  actionGroupIDs: string[] | null;
  inTrash: boolean;
  schedule: number;
  type: number;
  todayIndex: number;
  todayIndexRefDate: number | null;
  dueOrder: number;
  startBucket: number;
  alarmTimeOffset: number | null;
  tagIDs: string[] | null;
  recurrenceIDs: string[] | null;
  delegateIDs: string[] | null;
  repeater: Repeater | null;
}

export interface Repeater {
  ia?: number | null;
  rc?: number | null;
  fu: number;
  fa: number;
  of: { dy?: number | null; mo?: number | null; wd?: number | null; wdo?: number | null }[] | null;
  ed?: number | null;
  rrv?: number;
  tp?: number;
  ts?: number;
  sr?: number | null;
}

export interface Area {
  uuid: string;
  title: string;
}

export interface Tag {
  uuid: string;
  title: string;
  parentTagIDs: string[] | null;
  shortHand: string;
}

export interface ChecklistItem {
  uuid: string;
  creationDate: number | null;
  modificationDate: number | null;
  status: number;
  title: string;
  index: number;
  completionDate: number | null;
  taskIDs: string[] | null;
}

export type EntityKind = "task" | "area" | "tag" | "checklist";

export class State {
  tasks = new Map<string, Task>();
  areas = new Map<string, Area>();
  tags = new Map<string, Tag>();
  checklist = new Map<string, ChecklistItem>();

  // Set by update(): entities touched since the last call to takeDirty().
  dirty = new Map<string, { kind: EntityKind; uuid: string }>();

  private mark(kind: EntityKind, uuid: string) {
    this.dirty.set(kind + ":" + uuid, { kind, uuid });
  }

  takeDirty() {
    const d = [...this.dirty.values()];
    this.dirty.clear();
    return d;
  }

  // Validates every item first, then applies them in order. A validation error
  // leaves the state untouched.
  update(items: RawItem[]): void {
    for (const it of items) {
      if (isIgnoredKind(it.e)) continue;
      if (it.t !== 0 && it.t !== 1 && it.t !== 2) throw new ThingsError(`item ${it.uuid} (${it.e}) has unsupported action ${it.t}`);
      const kind = kindOf(it.e);
      if (!kind) throw new ThingsError(`item ${it.uuid} has unsupported kind "${it.e}"`);
      const err = validatePayload(kind, it.p);
      if (err) throw new ThingsError(`decode item ${it.uuid} (${it.e}): ${err}`);
    }
    for (const raw of items) {
      if (isIgnoredKind(raw.e)) continue;
      const legacy = LEGACY_KINDS.has(raw.e);
      let uuid = raw.uuid;
      if (legacy && !validUUID(uuid)) uuid = encodeLegacyIdentifier(uuid);
      const kind = kindOf(raw.e)!;
      const p = raw.p && typeof raw.p === "object" ? raw.p : {};
      if (kind === "tombstone") {
        let oid = typeof p.dloid === "string" ? p.dloid : "";
        if (legacy && !validUUID(oid)) oid = encodeLegacyIdentifier(oid);
        for (const k of ["task", "area", "tag", "checklist"] as EntityKind[]) {
          if (this.mapFor(k).delete(oid)) this.mark(k, oid);
        }
        continue;
      }
      const map = this.mapFor(kind);
      if (raw.t === 2) {
        map.delete(uuid);
        this.mark(kind, uuid);
        continue;
      }
      const enc = (ids: any) => (legacy && Array.isArray(ids) ? ids.map((id: string) => (validUUID(id) ? id : encodeLegacyIdentifier(id))) : ids);
      if (kind === "task") this.tasks.set(uuid, applyTask(this.tasks.get(uuid), uuid, p, enc));
      else if (kind === "checklist") this.checklist.set(uuid, applyChecklist(this.checklist.get(uuid), uuid, p, enc));
      else if (kind === "area") this.areas.set(uuid, applyArea(this.areas.get(uuid), uuid, p));
      else this.tags.set(uuid, applyTag(this.tags.get(uuid), uuid, p, enc));
      this.mark(kind, uuid);
    }
  }

  mapFor(kind: EntityKind): Map<string, any> {
    return kind === "task" ? this.tasks : kind === "area" ? this.areas : kind === "tag" ? this.tags : this.checklist;
  }

  get(kind: EntityKind, uuid: string): any {
    return this.mapFor(kind).get(uuid);
  }
}

// Records that carry no task-graph state: versioned account settings, and the
// Mail to Things queue (Command holds a forwarded email until a Things app turns
// it into an ordinary task; Command3 is the queue entry's deletion). Command is
// matched by exact version so a future Command4 still fails closed.
export function isIgnoredKind(kind: string): boolean {
  return isSettingsKind(kind) || kind === "Command" || kind === "Command3";
}

export function isSettingsKind(kind: string): boolean {
  return /^Settings\d+$/.test(kind);
}

const LEGACY_KINDS = new Set(["Task4", "Task3", "Task2", "Task", "ChecklistItem", "ChecklistItem2", "Area2", "Area", "Tag3", "Tag2", "Tag", "Tombstone"]);

function kindOf(e: string): EntityKind | "tombstone" | null {
  switch (e) {
    case "Task6":
    case "Task7":
    case "Task4":
    case "Task3":
    case "Task2":
    case "Task":
      return "task";
    case "ChecklistItem":
    case "ChecklistItem2":
    case "ChecklistItem3":
      return "checklist";
    case "Area2":
    case "Area3":
    case "Area":
      return "area";
    case "Tag3":
    case "Tag4":
    case "Tag2":
    case "Tag":
      return "tag";
    case "Tombstone2":
    case "Tombstone":
      return "tombstone";
  }
  return null;
}

// ---------------------------------------------------------------------------
// Payload validation: the same type checks Go's json.Unmarshal performed into
// the SDK's typed payload structs. null is always accepted (nil pointer).
// ---------------------------------------------------------------------------

type Check = (v: any) => boolean;
const isNull = (v: any) => v === null || v === undefined;
const int: Check = (v) => isNull(v) || Number.isInteger(v);
const num: Check = (v) => isNull(v) || (typeof v === "number" && Number.isFinite(v));
const str: Check = (v) => isNull(v) || typeof v === "string";
const bool: Check = (v) => isNull(v) || typeof v === "boolean";
const strs: Check = (v) => isNull(v) || (Array.isArray(v) && v.every((x) => typeof x === "string"));
const any: Check = () => true;
const obj = (fields: Record<string, Check>): Check => (v) =>
  isNull(v) || (typeof v === "object" && !Array.isArray(v) && Object.entries(fields).every(([k, c]) => c(v[k])));
const arr = (c: Check): Check => (v) => isNull(v) || (Array.isArray(v) && v.every(c));

const repeaterCheck = obj({
  ia: num, rc: int, fu: int, fa: int, ed: num, rrv: int, tp: int, ts: int, sr: num,
  of: arr(obj({ dy: int, mo: int, wd: int, wdo: int })),
});

const SCHEMAS: Record<string, Record<string, Check>> = {
  task: {
    ix: int, cd: num, md: num, sr: num, sp: num, dd: num, tir: num, ss: int, tp: int, tt: str, nt: any,
    ar: strs, pr: strs, tg: strs, tr: bool, ti: int, rt: strs, st: int, agr: strs, rr: repeaterCheck,
    do: int, lt: bool, icp: bool, icc: int, icsd: num, sb: int, dl: strs, lai: num, rmd: num, ato: int,
    acrd: num, dds: num, xx: any,
  },
  checklist: { cd: num, md: num, ix: int, ss: int, tt: str, sp: num, ts: strs, lt: bool, xx: any },
  area: { ix: int, tt: str, tg: strs },
  tag: { ix: int, tt: str, sh: str, pn: strs, xx: any },
  tombstone: { dloid: str, dld: num },
};

function validatePayload(kind: string, p: any): string | null {
  if (p === null || p === undefined) return null;
  if (typeof p !== "object" || Array.isArray(p)) return "payload is not an object";
  for (const [k, check] of Object.entries(SCHEMAS[kind])) {
    if (!check(p[k])) return `field ${k} has an invalid value`;
  }
  return null;
}

// ---------------------------------------------------------------------------
// Applying payloads
// ---------------------------------------------------------------------------

function has(p: any, k: string): boolean {
  return p[k] !== undefined && p[k] !== null;
}

function isNullField(p: any, k: string): boolean {
  return k in p && p[k] === null;
}

function applyTask(prev: Task | undefined, uuid: string, p: any, enc: (ids: any) => any): Task {
  const t: Task = prev ?? {
    uuid, creationDate: null, modificationDate: null, status: 0, title: "", note: "", scheduledDate: null,
    completionDate: null, deadlineDate: null, index: 0, areaIDs: null, parentTaskIDs: null, actionGroupIDs: null,
    inTrash: false, schedule: 1, type: 0, todayIndex: 0, todayIndexRefDate: null, dueOrder: 0, startBucket: 0,
    alarmTimeOffset: null, tagIDs: null, recurrenceIDs: null, delegateIDs: null, repeater: null,
  };
  t.uuid = uuid;
  if (has(p, "tt")) t.title = p.tt;
  if (has(p, "tp")) t.type = p.tp;
  if (has(p, "ss")) t.status = p.ss;
  if (has(p, "ix")) t.index = p.ix;
  if (has(p, "tr")) t.inTrash = p.tr;
  if (has(p, "st")) t.schedule = p.st;
  if (has(p, "sr")) t.scheduledDate = p.sr;
  if (has(p, "sp")) t.completionDate = p.sp;
  if (has(p, "dd")) t.deadlineDate = p.dd;
  if (has(p, "cd")) t.creationDate = p.cd;
  if (has(p, "md")) t.modificationDate = p.md;
  if (has(p, "ar")) t.areaIDs = enc(p.ar);
  if (has(p, "agr")) t.actionGroupIDs = enc(p.agr);
  if (has(p, "pr")) t.parentTaskIDs = enc(p.pr);
  if ("nt" in p) t.note = applyNote(t.note, p.nt);
  if (has(p, "sb")) t.startBucket = p.sb;
  if (has(p, "ato")) t.alarmTimeOffset = p.ato;
  if (has(p, "tg")) t.tagIDs = enc(p.tg);
  if (has(p, "do")) t.dueOrder = p.do;
  if (has(p, "ti")) t.todayIndex = p.ti;
  if (has(p, "dl")) t.delegateIDs = enc(p.dl);
  if (has(p, "rt")) t.recurrenceIDs = enc(p.rt);
  if (has(p, "tir")) t.todayIndexRefDate = p.tir;
  if (has(p, "rr")) t.repeater = p.rr;
  if (isNullField(p, "sr")) t.scheduledDate = null;
  if (isNullField(p, "dd")) t.deadlineDate = null;
  if (isNullField(p, "sp")) t.completionDate = null;
  if (isNullField(p, "md")) t.modificationDate = null;
  if (isNullField(p, "ato")) t.alarmTimeOffset = null;
  if (isNullField(p, "tir")) t.todayIndexRefDate = null;
  if (isNullField(p, "nt")) t.note = "";
  if (isNullField(p, "tg")) t.tagIDs = null;
  if (isNullField(p, "rr")) t.repeater = null;
  return t;
}

function applyNote(current: string, nt: any): string {
  if (nt === null) return "";
  if (typeof nt === "string") return nt;
  if (typeof nt !== "object" || Array.isArray(nt)) return current;
  // Mirror Go: a note object that does not decode cleanly leaves the note as is.
  const type = nt.t ?? 0;
  if (!Number.isInteger(type) || !str(nt._t) || !int(nt.ch) || !str(nt.v)) return current;
  const patches = nt.ps ?? [];
  if (!Array.isArray(patches) || !patches.every(obj({ r: str, p: int, l: int, ch: int }))) return current;
  if (type === 1) return nt.v ?? "";
  if (type === 2) return applyPatches(current, patches);
  return current;
}

export function applyPatches(original: string, patches: any[]): string {
  let runes = Array.from(original);
  for (const p of patches) {
    let pos = Number.isInteger(p?.p) ? p.p : 0;
    const len = Number.isInteger(p?.l) ? p.l : 0;
    const repl: string = typeof p?.r === "string" ? p.r : "";
    if (pos < 0) pos = 0;
    if (pos > runes.length) pos = runes.length;
    let end = pos + len;
    if (end > runes.length) end = runes.length;
    if (end < pos) end = pos;
    runes = [...runes.slice(0, pos), ...Array.from(repl), ...runes.slice(end)];
  }
  return runes.join("");
}

function applyChecklist(prev: ChecklistItem | undefined, uuid: string, p: any, enc: (ids: any) => any): ChecklistItem {
  const c: ChecklistItem = prev ?? { uuid, creationDate: null, modificationDate: null, status: 0, title: "", index: 0, completionDate: null, taskIDs: null };
  c.uuid = uuid;
  if (has(p, "cd")) c.creationDate = p.cd;
  if (has(p, "md")) c.modificationDate = p.md;
  if (has(p, "ix")) c.index = p.ix;
  if (has(p, "tt")) c.title = p.tt;
  if (has(p, "ss")) c.status = p.ss;
  if (has(p, "ts")) c.taskIDs = enc(p.ts);
  if (has(p, "sp")) c.completionDate = p.sp;
  if (isNullField(p, "md")) c.modificationDate = null;
  if (isNullField(p, "sp")) c.completionDate = null;
  return c;
}

function applyArea(prev: Area | undefined, uuid: string, p: any): Area {
  const a: Area = prev ?? { uuid, title: "" };
  a.uuid = uuid;
  if (has(p, "tt")) a.title = p.tt;
  return a;
}

function applyTag(prev: Tag | undefined, uuid: string, p: any, enc: (ids: any) => any): Tag {
  const t: Tag = prev ?? { uuid, title: "", parentTagIDs: null, shortHand: "" };
  t.uuid = uuid;
  if (has(p, "tt")) t.title = p.tt;
  if (has(p, "sh")) t.shortHand = p.sh;
  if (has(p, "pn")) t.parentTagIDs = enc(p.pn);
  return t;
}

// ---------------------------------------------------------------------------
// Dates (UTC calendar arithmetic with Go's AddDate normalization)
// ---------------------------------------------------------------------------

export function ymd(sec: number): { y: number; m: number; d: number; wd: number } {
  const dt = new Date(Math.floor(sec * 1000));
  return { y: dt.getUTCFullYear(), m: dt.getUTCMonth(), d: dt.getUTCDate(), wd: dt.getUTCDay() };
}

export function utcMidnight(y: number, m: number, d: number): number {
  const dt = new Date(0);
  dt.setUTCFullYear(y, m, d);
  dt.setUTCHours(0, 0, 0, 0);
  return dt.getTime() / 1000;
}

export function addDate(sec: number, years: number, months: number, days: number): number {
  const dt = new Date(Math.floor(sec * 1000));
  const frac = sec * 1000 - Math.floor(sec * 1000);
  const out = new Date(0);
  out.setUTCFullYear(dt.getUTCFullYear() + years, dt.getUTCMonth() + months, dt.getUTCDate() + days);
  out.setUTCHours(dt.getUTCHours(), dt.getUTCMinutes(), dt.getUTCSeconds(), dt.getUTCMilliseconds());
  return (out.getTime() + frac) / 1000;
}

export function isNeverending(rr: Repeater): boolean {
  return rr.ed !== null && rr.ed !== undefined && ymd(rr.ed).y === 4001;
}

// NextOccurrenceAfter from the SDK. `current` is reduced to its calendar date
// (in the local calendar it was given in) at UTC midnight.
export function nextOccurrenceAfter(rr: Repeater, current: { y: number; m: number; d: number; wd: number }): number | null {
  const base = utcMidnight(current.y, current.m, current.d);
  const fa = rr.fa;
  switch (rr.fu) {
    case 16:
      return addDate(base, 0, 0, fa);
    case 256: {
      const of = rr.of ?? [];
      if (of.length <= 1) return addDate(base, 0, 0, fa * 7);
      const wds = of.filter((d) => d.wd !== null && d.wd !== undefined).map((d) => d.wd as number);
      wds.sort((a, b) => a - b);
      const cur = current.wd;
      for (const w of wds) if (w > cur) return addDate(base, 0, 0, w - cur);
      return addDate(base, 0, 0, 7 * fa - cur + wds[0]);
    }
    case 8:
      return addDate(base, 0, fa, 0);
    case 4:
      return addDate(base, fa, 0, 0);
  }
  return null;
}
