// The 23 MCP tools. Handlers read the account's State and write through
// Ctx.write, which validates, syncs, commits and applies (see session.ts).
// Ported one-to-one from the Go server's main.go.

import {
  State, Task, Repeater, newUUID, ymd, utcMidnight, isNeverending, nextOccurrenceAfter, RawItem,
} from "./things";

export interface Envelope {
  uuid: string;
  t: number;
  e: string;
  p: any;
}

export interface Ctx {
  state: State;
  write(envs: Envelope[]): Promise<void>;
}

export type Args = Record<string, any>;

export class ToolError extends Error {}

// ---------------------------------------------------------------------------
// Argument access (mcp-go GetString/GetBool/GetInt semantics)
// ---------------------------------------------------------------------------

function getString(a: Args, k: string, def = ""): string {
  return typeof a[k] === "string" ? a[k] : def;
}

function getBool(a: Args, k: string, def = false): boolean {
  const v = a[k];
  if (typeof v === "boolean") return v;
  if (typeof v === "string") {
    if (["1", "t", "T", "TRUE", "true", "True"].includes(v)) return true;
    if (["0", "f", "F", "FALSE", "false", "False"].includes(v)) return false;
    return def;
  }
  if (typeof v === "number") return v !== 0;
  return def;
}

function getInt(a: Args, k: string, def: number): number {
  const v = a[k];
  if (typeof v === "number") return Math.trunc(v);
  if (typeof v === "string" && /^[+-]?\d+$/.test(v)) return parseInt(v, 10);
  return def;
}

function requireString(a: Args, k: string): string {
  if (typeof a[k] !== "string") throw new ToolError(`${k} is required`);
  return a[k];
}

function requireNonEmpty(a: Args, k: string): string {
  const v = a[k];
  if (typeof v !== "string" || v.trim() === "") throw new ToolError(`${k} is required and must not be empty`);
  return v.trim();
}

// ---------------------------------------------------------------------------
// Dates
// ---------------------------------------------------------------------------

interface ParsedDate {
  sec: number; // Unix seconds (floored, like Go's Unix())
  local: { y: number; m: number; d: number; wd: number }; // calendar date in its own offset
}

function daysIn(y: number, m: number): number {
  return new Date(Date.UTC(y, m + 1, 0)).getUTCDate();
}

// RFC3339 first, then YYYY-MM-DD (UTC midnight), like Go's parseDate.
export function parseDate(s: string): ParsedDate | null {
  let m = /^(\d{4})-(\d{2})-(\d{2})[Tt](\d{2}):(\d{2}):(\d{2})(\.\d+)?([Zz]|([+-])(\d{2}):(\d{2}))$/.exec(s);
  if (m) {
    const [y, mo, d, h, mi, se] = [+m[1], +m[2] - 1, +m[3], +m[4], +m[5], +m[6]];
    if (mo > 11 || d < 1 || d > daysIn(y, mo) || h > 23 || mi > 59 || se > 59) return null;
    let off = 0;
    if (m[9]) {
      const oh = +m[10], om = +m[11];
      if (oh > 23 || om > 59) return null;
      off = (m[9] === "-" ? -1 : 1) * (oh * 60 + om);
    }
    const sec = Math.floor((Date.UTC(y, mo, d, h, mi, se) - off * 60000) / 1000);
    return { sec, local: { y, m: mo, d, wd: new Date(Date.UTC(y, mo, d)).getUTCDay() } };
  }
  m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s);
  if (!m) return null;
  const [y, mo, d] = [+m[1], +m[2] - 1, +m[3]];
  if (mo > 11 || d < 1 || d > daysIn(y, mo)) return null;
  const sec = Date.UTC(y, mo, d) / 1000;
  return { sec, local: { y, m: mo, d, wd: new Date(Date.UTC(y, mo, d)).getUTCDay() } };
}

function parseTime(s: string): number | null {
  const m = /^([+-]?\d+):([+-]?\d+)$/.exec(s);
  if (!m) return null;
  const h = parseInt(m[1], 10), mi = parseInt(m[2], 10);
  if (h < 0 || h > 23 || mi < 0 || mi > 59) return null;
  return h * 3600 + mi * 60;
}

function offsetToTime(secs: number): string {
  const h = Math.trunc(secs / 3600), m = Math.trunc((secs % 3600) / 60);
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
}

function nowTs(): number {
  return Date.now() / 1000;
}

function nowLocal(): ParsedDate["local"] {
  const t = ymd(nowTs());
  return t;
}

function todayMidnightUTC(): number {
  const t = ymd(nowTs());
  return utcMidnight(t.y, t.m, t.d);
}

function dateStr(sec: number): string {
  return new Date(Math.floor(sec * 1000)).toISOString().slice(0, 10);
}

function isoStr(sec: number): string {
  return new Date(Math.floor(sec) * 1000).toISOString().slice(0, 19) + "Z";
}

function after1970(sec: number | null): sec is number {
  return sec !== null && ymd(sec).y > 1970;
}

// ---------------------------------------------------------------------------
// Wire payloads
// ---------------------------------------------------------------------------

function crc32(s: string): number {
  let c = 0xffffffff;
  for (const b of new TextEncoder().encode(s)) {
    c ^= b;
    for (let k = 0; k < 8; k++) c = c & 1 ? (c >>> 1) ^ 0xedb88320 : c >>> 1;
  }
  return (c ^ 0xffffffff) >>> 0;
}

const emptyNote = () => ({ _t: "tx", ch: 0, v: "", t: 1 });
const textNote = (s: string) => ({ _t: "tx", ch: crc32(s), v: s, t: 1 });
const defaultExtension = () => ({ sn: {}, _t: "oo" });

export function parseRecurrence(input: string, ref: ParsedDate["local"]): any | null {
  const s = input.trim().toLowerCase();
  if (s === "" || s === "none") return null;
  const refSec = utcMidnight(ref.y, ref.m, ref.d);
  const base: any = { rrv: 4, tp: 0, rc: 0, ts: 0, ed: 64092211200, ia: refSec, sr: refSec };
  let m: RegExpExecArray | null;
  const positive = (n: string) => {
    if (!/^[+-]?\d+$/.test(n)) return null;
    const v = parseInt(n, 10);
    return v >= 1 ? v : null;
  };
  if (s === "daily") Object.assign(base, { fu: 16, fa: 1, of: [{ dy: 0 }] });
  else if ((m = /^every (.*) days$/.exec(s))) {
    const n = positive(m[1]);
    if (n === null) throw new ToolError(`invalid recurrence: ${s}`);
    Object.assign(base, { fu: 16, fa: n, of: [{ dy: 0 }] });
  } else if (s === "weekly") Object.assign(base, { fu: 256, fa: 1, of: [{ wd: ref.wd }] });
  else if (s.startsWith("weekly:")) Object.assign(base, { fu: 256, fa: 1, of: parseWeekdays(s.slice(7)) });
  else if ((m = /^every (.*) weeks$/.exec(s))) {
    const n = positive(m[1]);
    if (n === null) throw new ToolError(`invalid recurrence: ${s}`);
    Object.assign(base, { fu: 256, fa: n, of: [{ wd: ref.wd }] });
  } else if (s === "monthly") Object.assign(base, { fu: 8, fa: 1, of: [{ dy: 0 }] });
  else if (s.startsWith("monthly:")) {
    const detail = s.slice(8);
    if (detail === "last") Object.assign(base, { fu: 8, fa: 1, of: [{ dy: -1 }] });
    else {
      const day = /^[+-]?\d+$/.test(detail) ? parseInt(detail, 10) : NaN;
      if (!(day >= 1 && day <= 31)) throw new ToolError(`invalid monthly day: ${detail}`);
      Object.assign(base, { fu: 8, fa: 1, of: [{ dy: day - 1 }] });
    }
  } else if (s === "yearly") Object.assign(base, { fu: 4, fa: 1, of: [{ dy: 0, mo: 0 }] });
  else
    throw new ToolError(
      `unsupported recurrence format: ${s} (try: daily, weekly, weekly:mon,wed, monthly, monthly:15, monthly:last, yearly, every N days, every N weeks)`,
    );
  return base;
}

function parseWeekdays(s: string): any[] {
  const map: Record<string, number> = { sun: 0, mon: 1, tue: 2, wed: 3, thu: 4, fri: 5, sat: 6 };
  const out: any[] = [];
  for (const raw of s.split(",")) {
    const p = raw.trim();
    if (!(p in map)) throw new ToolError(`unknown weekday: ${p} (use: sun,mon,tue,wed,thu,fri,sat)`);
    out.push({ wd: map[p] });
  }
  if (out.length === 0) throw new ToolError("no weekdays specified");
  return out;
}

function splitUUIDs(csv: string): string[] {
  return csv.split(",").map((s) => s.trim()).filter((s) => s !== "");
}

type Opts = Record<string, string>;

function newTaskCreatePayload(title: string, opts: Opts, ix: number): any {
  const now = nowTs();
  let st = 0, tp = 0, sb = 0;
  let sr: number | null = null, tir: number | null = null, dd: number | null = null;
  let pr: string[] = [], agr: string[] = [], ar: string[] = [], tg: string[] = [];
  let nt: any = emptyNote();
  if ("type" in opts) {
    if (opts.type === "project") { tp = 1; st = 1; }
    else if (opts.type === "heading") { tp = 2; st = 1; }
  }
  if ("schedule" in opts) {
    const v = opts.schedule;
    if (v === "today") { st = 1; sr = tir = todayMidnightUTC(); }
    else if (v === "tonight") { st = 1; sr = tir = todayMidnightUTC(); sb = 1; }
    else if (v === "anytime") st = 1;
    else if (v === "someday") st = 2;
    else if (v === "inbox") st = 0;
    else {
      const t = parseDate(v);
      if (t) { sr = tir = t.sec; st = 2; }
    }
  }
  if (opts.note) nt = textNote(opts.note);
  if ("deadline" in opts) {
    const t = parseDate(opts.deadline);
    if (t) dd = t.sec;
  }
  for (const [key, set] of [["project_uuid", (v: string) => (pr = [v])], ["heading_uuid", (v: string) => (agr = [v])], ["area_uuid", (v: string) => (ar = [v])]] as const) {
    if (opts[key]) {
      set(opts[key]);
      if (!("schedule" in opts)) st = 1;
    }
  }
  if (opts.tags) tg = splitUUIDs(opts.tags);
  let rmd: number | null = null, ato: number | null = null;
  if ("reminder_date" in opts && "reminder_time" in opts) {
    const dt = parseDate(opts.reminder_date);
    const off = parseTime(opts.reminder_time);
    if (dt && off !== null) { rmd = dt.sec; ato = off; }
  }
  let rr: any = null, icsd: number | null = null;
  if (opts.recurrence) {
    let ref = nowLocal();
    if (opts.schedule) {
      const dt = parseDate(opts.schedule);
      if (dt) ref = dt.local;
    }
    try {
      const parsed = parseRecurrence(opts.recurrence, ref);
      if (parsed) { rr = parsed; icsd = todayMidnightUTC(); }
    } catch {}
  }
  return {
    tp, sr, dds: null, rt: [], rmd, ss: 0, tr: false, dl: [], icp: false, st, ar, tt: title, do: 0, lai: null, tir,
    tg, agr, ix, cd: now, lt: false, icc: 0, md: null, ti: 0, dd, ato, nt, icsd, pr, rp: null, acrd: null, sp: null,
    sb, rr, xx: defaultExtension(),
  };
}

function splitRecurringPayload(payload: any): { templateUUID: string; template: any; instance: any } {
  let refSec = nowTs();
  if (payload.tir !== null) refSec = payload.tir;
  else if (payload.sr !== null) refSec = payload.sr;
  const next = nextOccurrenceAfter(payload.rr as Repeater, ymd(refSec));
  if (next === null) throw new ToolError("could not compute next recurrence date");
  const templateUUID = newUUID();
  const template = { ...payload, rr: payload.rr, icsd: todayMidnightUTC(), rt: [], sr: next, tir: next, st: 1, sb: 0 };
  const instance = { ...payload, rr: null, icsd: null, rt: [templateUUID] };
  return { templateUUID, template, instance };
}

function recurringTemplateForEdit(task: Task, a: Args, rr: any, next: number): any {
  const title = getString(a, "title") || task.title;
  const note = getString(a, "note") || task.note;
  // Go's append([]string(nil), ...) is nil for an empty list, so it goes out as null.
  const copy = (l: string[] | null) => (l && l.length ? [...l] : null);
  let areas = copy(task.areaIDs), projects = copy(task.parentTaskIDs), headings = copy(task.actionGroupIDs);
  if (getString(a, "area_uuid")) [areas, projects, headings] = [[getString(a, "area_uuid")], [], []] as (string[] | null)[];
  if (getString(a, "project_uuid")) [areas, projects, headings] = [[], [getString(a, "project_uuid")], []] as (string[] | null)[];
  if (getString(a, "heading_uuid")) [areas, projects, headings] = [[], [], [getString(a, "heading_uuid")]] as (string[] | null)[];
  let tags = copy(task.tagIDs);
  if (getString(a, "tags")) tags = splitUUIDs(getString(a, "tags"));
  let deadline: number | null = task.deadlineDate !== null ? Math.floor(task.deadlineDate) : null;
  const dv = getString(a, "deadline");
  if (dv === "none") deadline = null;
  else if (dv) {
    const p = parseDate(dv);
    if (p) deadline = p.sec;
  }
  return {
    tp: task.type, sr: next, dds: null, rt: [], rmd: null, ss: 0, tr: false, dl: copy(task.delegateIDs), icp: false, st: 1,
    ar: areas, tt: title, do: task.dueOrder, lai: null, tir: next, tg: tags, agr: headings, ix: task.index, cd: nowTs(),
    lt: false, icc: 0, md: null, ti: 0, dd: deadline, ato: task.alarmTimeOffset, nt: textNote(note), icsd: todayMidnightUTC(),
    pr: projects, rp: null, acrd: null, sp: null, sb: 0, rr, xx: defaultExtension(),
  };
}

// ---------------------------------------------------------------------------
// Graph helpers
// ---------------------------------------------------------------------------

const contains = (list: string[] | null, s: string) => (list ?? []).includes(s);
const sortByIndex = (tasks: Task[]) => tasks.sort((a, b) => a.index - b.index);
const isTemplate = (t: Task) => t.repeater !== null;
// Shells of tasks created and abandoned without typing anything. A blank title
// with a note (a link saved from a share sheet) is a real task.
const isUntitled = (t: Task) => t.title.trim() === "" && t.note.trim() === "";

function statusString(s: number): string {
  return s === 3 ? "completed" : s === 2 ? "canceled" : "pending";
}

function isTodayDate(sec: number): boolean {
  const a = ymd(sec), b = ymd(nowTs());
  return a.y === b.y && a.m === b.m && a.d === b.d;
}

function scheduleString(st: number, scheduled: number | null, sb: number): string {
  if (st === 1) {
    if (scheduled !== null && isTodayDate(scheduled)) return sb === 1 ? "tonight" : "today";
    return "anytime";
  }
  if (st === 2) return scheduled !== null ? "upcoming" : "someday";
  return "inbox";
}

const effectiveScheduled = (t: Task) => (t.todayIndexRefDate !== null ? t.todayIndexRefDate : t.scheduledDate);

function todayEnd(): number {
  const n = ymd(nowTs());
  return utcMidnight(n.y, n.m, n.d) + 86399;
}

function isScheduledForTodayOrPast(t: Task): boolean {
  if (t.schedule !== 1 && t.schedule !== 2) return false;
  const d = effectiveScheduled(t);
  return d !== null && d <= todayEnd();
}

function taskScheduleString(t: Task): string {
  if (t.status === 0 && isScheduledForTodayOrPast(t)) return t.startBucket === 1 ? "tonight" : "today";
  return scheduleString(t.schedule, effectiveScheduled(t), t.startBucket);
}

class Graph {
  constructor(public s: State) {}

  templateFor(t: Task): Task | null {
    if (!t.recurrenceIDs || t.recurrenceIDs.length === 0) return null;
    const tpl = this.s.tasks.get(t.recurrenceIDs[0]);
    return tpl && isTemplate(tpl) ? tpl : null;
  }

  effectiveTagIDs(t: Task): string[] {
    if (t.tagIDs && t.tagIDs.length > 0) return t.tagIDs;
    return this.templateFor(t)?.tagIDs ?? [];
  }

  effectiveAreaIDs(t: Task): string[] {
    if (t.areaIDs && t.areaIDs.length > 0) return t.areaIDs;
    return this.templateFor(t)?.areaIDs ?? [];
  }

  taskMatchesProject(t: Task, projectUUID: string): boolean {
    if (contains(t.parentTaskIDs, projectUUID)) return true;
    if (t.actionGroupIDs && t.actionGroupIDs.length > 0) {
      const h = this.s.tasks.get(t.actionGroupIDs[0]);
      if (h) return contains(h.parentTaskIDs, projectUUID);
    }
    return false;
  }

  taskMatchesArea(t: Task, areaUUID: string): boolean {
    if (this.effectiveAreaIDs(t).includes(areaUUID)) return true;
    let projectUUID = "";
    if (t.parentTaskIDs && t.parentTaskIDs.length > 0) projectUUID = t.parentTaskIDs[0];
    else if (t.actionGroupIDs && t.actionGroupIDs.length > 0) {
      const h = this.s.tasks.get(t.actionGroupIDs[0]);
      if (h && h.parentTaskIDs && h.parentTaskIDs.length > 0) projectUUID = h.parentTaskIDs[0];
    }
    if (projectUUID) {
      const p = this.s.tasks.get(projectUUID);
      if (p) return contains(p.areaIDs, areaUUID);
    }
    return false;
  }

  // Every container in the chain must be pending (and outside Trash unless
  // includeTrash). Missing references and cycles fail closed.
  hasPendingAncestors(task: Task, includeTrash: boolean): boolean {
    const visiting = new Set([task.uuid]);
    const visit = (item: Task): boolean => {
      for (const id of [...(item.parentTaskIDs ?? []), ...(item.actionGroupIDs ?? [])]) {
        const c = this.s.tasks.get(id);
        if (!c || c.status !== 0 || (!includeTrash && c.inTrash)) return false;
        if (visiting.has(id)) return false;
        visiting.add(id);
        if (!visit(c)) return false;
        visiting.delete(id);
      }
      return true;
    };
    return visit(task);
  }

  isEffectivelyActive(t: Task): boolean {
    return t.status === 0 && !t.inTrash && this.hasPendingAncestors(t, false);
  }

  projectedOccurrence(tpl: Task): Task | null {
    if (!isTemplate(tpl) || tpl.inTrash || isUntitled(tpl)) return null;
    const next = tpl.todayIndexRefDate;
    if (next === null) return null;
    const rr = tpl.repeater!;
    if (!isNeverending(rr) && rr.ed !== null && rr.ed !== undefined && rr.ed < next) return null;
    for (const t of this.s.tasks.values()) {
      if (t.status === 0 && !t.inTrash && !isTemplate(t) && contains(t.recurrenceIDs, tpl.uuid)) return null;
    }
    return { ...tpl, repeater: null, recurrenceIDs: [tpl.uuid], status: 0, schedule: 2, scheduledDate: next, completionDate: null };
  }

  projectedOccurrences(): Task[] {
    const out: Task[] = [];
    for (const t of this.s.tasks.values()) {
      if (!isTemplate(t)) continue;
      const p = this.projectedOccurrence(t);
      if (p) out.push(p);
    }
    return out;
  }

  // Candidates for list tools: every task plus projected occurrences.
  candidates(): { list: Task[]; projected: Set<Task> } {
    const list = [...this.s.tasks.values()];
    const projected = new Set<Task>();
    for (const p of this.projectedOccurrences()) {
      list.push(p);
      projected.add(p);
    }
    return { list, projected };
  }

  toOutput(t: Task, projected = false): any {
    const s = this.s;
    const eff = effectiveScheduled(t);
    const out: any = { uuid: t.uuid, type: t.type === 1 ? "project" : "task", title: t.title };
    if (t.note) out.note = t.note;
    out.status = statusString(t.status);
    out.schedule = taskScheduleString(t);
    if (after1970(eff)) out.scheduledDate = dateStr(eff);
    if (after1970(t.deadlineDate)) out.deadlineDate = dateStr(t.deadlineDate);
    if (t.alarmTimeOffset !== null) out.reminderTime = offsetToTime(t.alarmTimeOffset);
    out.isRecurring = (t.recurrenceIDs?.length ?? 0) > 0 || t.repeater !== null;
    if (projected) out.materialized = false;
    if (after1970(t.creationDate)) out.creationDate = isoStr(t.creationDate);
    if (after1970(t.modificationDate)) out.modificationDate = isoStr(t.modificationDate);
    if (after1970(t.completionDate)) out.completionDate = isoStr(t.completionDate);
    const areas: any[] = [];
    for (const id of this.effectiveAreaIDs(t)) {
      const a = s.areas.get(id);
      if (a) areas.push({ uuid: id, name: a.title });
    }
    let project: any = null;
    if (t.parentTaskIDs && t.parentTaskIDs.length > 0) {
      const pid = t.parentTaskIDs[0];
      const p = s.tasks.get(pid);
      if (p) project = { uuid: pid, name: p.title };
    } else if (t.actionGroupIDs && t.actionGroupIDs.length > 0) {
      const h = s.tasks.get(t.actionGroupIDs[0]);
      if (h && h.parentTaskIDs && h.parentTaskIDs.length > 0) {
        const pid = h.parentTaskIDs[0];
        const p = s.tasks.get(pid);
        if (p) project = { uuid: pid, name: p.title };
      }
    }
    if (areas.length === 0 && project) {
      const p = s.tasks.get(project.uuid);
      for (const id of p?.areaIDs ?? []) {
        const a = s.areas.get(id);
        if (a) areas.push({ uuid: id, name: a.title });
      }
    }
    if (areas.length) out.areas = areas;
    if (project) out.project = project;
    const tags: any[] = [];
    for (const id of this.effectiveTagIDs(t)) {
      const tag = s.tags.get(id);
      if (tag) tags.push({ uuid: id, name: tag.title });
    }
    if (tags.length) out.tags = tags;
    return out;
  }

  findAreaUUID(name: string): string {
    for (const a of this.s.areas.values()) if (equalFold(a.title, name)) return a.uuid;
    return "";
  }

  findProjectUUID(name: string): string {
    for (const t of this.s.tasks.values()) if (t.type === 1 && equalFold(t.title, name)) return t.uuid;
    return "";
  }

  findTagUUID(name: string): string {
    for (const t of this.s.tags.values()) if (equalFold(t.title, name)) return t.uuid;
    return "";
  }

  minIndexWhere(match: (t: Task) => boolean): number {
    let min = 1 << 30, found = false;
    for (const t of this.s.tasks.values()) {
      if (match(t) && (!found || t.index < min)) {
        min = t.index;
        found = true;
      }
    }
    return found ? min : 1;
  }

  // validation -------------------------------------------------------------

  validateTask(uuid: string) {
    for (const t of this.s.tasks.values()) if (t.uuid === uuid && !t.inTrash) return;
    throw new ToolError(`task not found: ${uuid}`);
  }

  validateProject(uuid: string) {
    const t = this.s.tasks.get(uuid);
    if (!t || t.type !== 1) throw new ToolError(`project not found: ${uuid}`);
    if (t.inTrash) throw new ToolError(`project is in trash: ${uuid}`);
    if (t.status !== 0) throw new ToolError(`project is ${statusString(t.status)}: ${uuid}`);
  }

  validateHeading(uuid: string) {
    const t = this.s.tasks.get(uuid);
    if (!t || t.type !== 2) throw new ToolError(`heading not found: ${uuid}`);
    if (t.inTrash) throw new ToolError(`heading is in trash: ${uuid}`);
    if (t.status !== 0) throw new ToolError(`heading is ${statusString(t.status)}: ${uuid}`);
    if (!this.hasPendingAncestors(t, false)) throw new ToolError(`heading is inside an inactive project: ${uuid}`);
  }

  validateArea(uuid: string) {
    if (!this.s.areas.has(uuid)) throw new ToolError(`area not found: ${uuid}`);
  }

  validateTag(uuid: string) {
    if (!this.s.tags.has(uuid)) throw new ToolError(`tag not found: ${uuid}`);
  }

  validateTags(csv: string) {
    const ids = splitUUIDs(csv);
    if (ids.length === 0) throw new ToolError("at least one tag UUID is required");
    for (const id of ids) if (!this.s.tags.has(id)) throw new ToolError(`tag not found: ${id}`);
  }

  validateChecklist(uuid: string) {
    if (!this.s.checklist.has(uuid)) throw new ToolError(`checklist item not found: ${uuid}`);
  }

  validateTagParent(tagUUID: string, parentUUID: string) {
    if (!parentUUID) return;
    if (tagUUID && tagUUID === parentUUID) throw new ToolError(`tag cannot be its own parent: ${tagUUID}`);
    this.validateTag(parentUUID);
    const visited = new Set<string>();
    let cur = parentUUID;
    while (cur) {
      if (cur === tagUUID) throw new ToolError(`tag parent would create a cycle involving ${tagUUID}`);
      if (visited.has(cur)) throw new ToolError(`existing tag hierarchy contains a cycle at ${cur}`);
      visited.add(cur);
      const tag = this.s.tags.get(cur);
      if (!tag || !tag.parentTagIDs || tag.parentTagIDs.length === 0) break;
      cur = tag.parentTagIDs[0];
    }
  }

  validateOpts(opts: Opts) {
    if ("area_uuid" in opts && ("project_uuid" in opts || "heading_uuid" in opts)) {
      throw new ToolError("area_uuid cannot be combined with project_uuid or heading_uuid");
    }
    if (opts.project_uuid) this.validateProject(opts.project_uuid);
    if (opts.heading_uuid) {
      this.validateHeading(opts.heading_uuid);
      if (opts.project_uuid) {
        const h = this.s.tasks.get(opts.heading_uuid);
        if (!h || !contains(h.parentTaskIDs, opts.project_uuid)) {
          throw new ToolError(`heading ${opts.heading_uuid} does not belong to project ${opts.project_uuid}`);
        }
      }
    }
    if (opts.area_uuid) this.validateArea(opts.area_uuid);
    if (opts.tags) this.validateTags(opts.tags);
    if (opts.schedule && !["today", "tonight", "anytime", "someday", "inbox"].includes(opts.schedule) && !parseDate(opts.schedule)) {
      throw new ToolError(`invalid schedule: ${opts.schedule}`);
    }
    if (opts.deadline && !parseDate(opts.deadline)) throw new ToolError(`invalid deadline: ${opts.deadline}`);
    const hasRD = "reminder_date" in opts, hasRT = "reminder_time" in opts;
    if (hasRD !== hasRT) throw new ToolError("reminder_date and reminder_time must be provided together");
    if (hasRD) {
      if (!parseDate(opts.reminder_date)) throw new ToolError(`invalid reminder_date: ${opts.reminder_date}`);
      if (parseTime(opts.reminder_time) === null) throw new ToolError(`invalid reminder_time: ${opts.reminder_time}`);
    }
  }
}

// Go's strings.EqualFold for the common case.
function equalFold(a: string, b: string): boolean {
  return a === b || a.toLowerCase() === b.toLowerCase() || a.toUpperCase() === b.toUpperCase();
}

function nextTopIndexBelow(minIx: number): number {
  return minIx - 1 < 1 ? 1 : minIx - 1;
}

// ---------------------------------------------------------------------------
// Read handlers
// ---------------------------------------------------------------------------

function parseFilterDate(v: string): number | null {
  if (!v) return null;
  const d = parseDate(v);
  if (!d) throw new ToolError(`invalid date: ${v}`);
  return d.sec;
}

interface ListFilters {
  schedule: string;
  sb: number | null; sa: number | null; db: number | null; da: number | null; cb: number | null; ca: number | null;
  tagUUID: string;
  containsText: string;
}

function readFilters(a: Args): ListFilters {
  return {
    schedule: getString(a, "schedule"),
    sb: parseFilterDate(getString(a, "scheduled_before")),
    sa: parseFilterDate(getString(a, "scheduled_after")),
    db: parseFilterDate(getString(a, "deadline_before")),
    da: parseFilterDate(getString(a, "deadline_after")),
    cb: parseFilterDate(getString(a, "created_before")),
    ca: parseFilterDate(getString(a, "created_after")),
    tagUUID: "",
    containsText: getString(a, "contains_text").toLowerCase(),
  };
}

function statusMatches(t: Task, filter: string): boolean | null {
  if (filter === "completed") return t.status === 3;
  if (filter === "canceled") return t.status === 2;
  return null;
}

function passesCommon(g: Graph, t: Task, f: ListFilters): boolean {
  if (f.schedule) {
    if (f.schedule === "today") {
      if (!isScheduledForTodayOrPast(t)) return false;
    } else if (f.schedule === "tonight") {
      if (!isScheduledForTodayOrPast(t) || t.startBucket !== 1) return false;
    } else if (taskScheduleString(t) !== f.schedule) return false;
  }
  const eff = effectiveScheduled(t);
  if (f.sb !== null && (eff === null || !(eff < f.sb))) return false;
  if (f.sa !== null && (eff === null || !(eff > f.sa))) return false;
  if (f.db !== null && (t.deadlineDate === null || !(t.deadlineDate < f.db))) return false;
  if (f.da !== null && (t.deadlineDate === null || !(t.deadlineDate > f.da))) return false;
  const cd = t.creationDate ?? -Infinity;
  if (f.cb !== null && !(cd < f.cb)) return false;
  if (f.ca !== null && !(cd > f.ca)) return false;
  if (f.tagUUID && !g.effectiveTagIDs(t).includes(f.tagUUID)) return false;
  if (f.containsText && !t.title.toLowerCase().includes(f.containsText) && !t.note.toLowerCase().includes(f.containsText)) return false;
  return true;
}

function resolveNames(g: Graph, a: Args, f: ListFilters, withProject: boolean) {
  let areaUUID = "", projectUUID = "";
  const areaName = getString(a, "area"), projectName = withProject ? getString(a, "project") : "", tagName = getString(a, "tag");
  if (areaName) {
    areaUUID = g.findAreaUUID(areaName);
    if (!areaUUID) throw new ToolError(`area not found: ${areaName}`);
  }
  if (projectName) {
    projectUUID = g.findProjectUUID(projectName);
    if (!projectUUID) throw new ToolError(`project not found: ${projectName}`);
  }
  if (tagName) {
    f.tagUUID = g.findTagUUID(tagName);
    if (!f.tagUUID) throw new ToolError(`tag not found: ${tagName}`);
  }
  return { areaUUID, projectUUID };
}

function findTasks(g: Graph, a: Args) {
  const f = readFilters(a);
  const { areaUUID, projectUUID } = resolveNames(g, a, f, true);
  const inTrash = getBool(a, "in_trash");
  const statusFilter = getString(a, "status", "pending");
  const { list, projected } = g.candidates();
  const filtered: Task[] = [];
  for (const t of list) {
    if (t.type === 1 || t.type === 2 || isTemplate(t) || isUntitled(t)) continue;
    if (!inTrash && t.inTrash) continue;
    const sm = statusMatches(t, statusFilter);
    if (sm === false) continue;
    if (sm === null && (t.status !== 0 || !g.hasPendingAncestors(t, inTrash))) continue;
    if (!passesCommon(g, t, f)) continue;
    if (areaUUID && !g.taskMatchesArea(t, areaUUID)) continue;
    if (projectUUID && !g.taskMatchesProject(t, projectUUID)) continue;
    filtered.push(t);
  }
  if (f.schedule === "today" || f.schedule === "tonight") {
    filtered.sort((x, y) => {
      if (x.startBucket !== y.startBucket) return x.startBucket - y.startBucket;
      const ti = x.todayIndexRefDate, tj = y.todayIndexRefDate;
      if (ti === null && tj === null) return x.todayIndex - y.todayIndex;
      if (ti === null) return 1;
      if (tj === null) return -1;
      if (ti !== tj) return tj - ti;
      return x.todayIndex - y.todayIndex;
    });
  } else sortByIndex(filtered);
  return filtered.map((t) => g.toOutput(t, projected.has(t)));
}

function findProjects(g: Graph, a: Args) {
  const f = readFilters(a);
  const { areaUUID } = resolveNames(g, a, f, false);
  const inTrash = getBool(a, "in_trash");
  const statusFilter = getString(a, "status", "pending");
  const filtered: Task[] = [];
  for (const t of g.s.tasks.values()) {
    if (t.type !== 1 || isTemplate(t) || isUntitled(t)) continue;
    if (!inTrash && t.inTrash) continue;
    const sm = statusMatches(t, statusFilter);
    if (sm === false || (sm === null && t.status !== 0)) continue;
    if (!passesCommon(g, t, f)) continue;
    if (areaUUID && !contains(t.areaIDs, areaUUID)) continue;
    filtered.push(t);
  }
  return sortByIndex(filtered).map((t) => g.toOutput(t));
}

function showTask(g: Graph, a: Args) {
  const prefix = requireString(a, "uuid");
  for (let t of g.s.tasks.values()) {
    if (!t.uuid.startsWith(prefix)) continue;
    let projected = false;
    if (isTemplate(t)) {
      const p = g.projectedOccurrence(t);
      if (!p) throw new ToolError(`${t.uuid} is a recurring template; query its open occurrence via things_find_tasks instead`);
      t = p;
      projected = true;
    }
    const out = g.toOutput(t, projected);
    const checklist: any[] = [];
    for (const c of g.s.checklist.values()) {
      if (contains(c.taskIDs, t.uuid)) checklist.push({ uuid: c.uuid, title: c.title, status: statusString(c.status) });
    }
    if (checklist.length) out.checklist = checklist;
    return out;
  }
  throw new ToolError(`task not found: ${prefix}`);
}

function showProject(g: Graph, a: Args) {
  const projectUUID = requireString(a, "uuid");
  const statusFilter = getString(a, "status", "pending");
  const project = g.s.tasks.get(projectUUID);
  if (!project || project.type !== 1) throw new ToolError(`project not found: ${projectUUID}`);
  const headingRaw: Task[] = [];
  const headingTasks = new Map<string, Task[]>();
  for (const t of g.s.tasks.values()) {
    if (t.type === 2 && !t.inTrash && contains(t.parentTaskIDs, projectUUID)) {
      headingRaw.push(t);
      headingTasks.set(t.uuid, []);
    }
  }
  sortByIndex(headingRaw);
  const { list, projected } = g.candidates();
  const unfiled: Task[] = [];
  for (const t of list) {
    if (t.inTrash || t.type === 1 || t.type === 2 || isTemplate(t) || isUntitled(t)) continue;
    const sm = statusMatches(t, statusFilter);
    if (sm === false || (sm === null && t.status !== 0)) continue;
    if (!g.taskMatchesProject(t, projectUUID)) continue;
    const hid = (t.actionGroupIDs ?? []).find((id) => headingTasks.has(id));
    if (hid) headingTasks.get(hid)!.push(t);
    else unfiled.push(t);
  }
  const out = g.toOutput(project);
  out.headings = headingRaw.map((h) => ({ uuid: h.uuid, title: h.title, tasks: sortByIndex(headingTasks.get(h.uuid)!).map((t) => g.toOutput(t, projected.has(t))) }));
  out.unfiledTasks = sortByIndex(unfiled).map((t) => g.toOutput(t, projected.has(t)));
  return out;
}

function findHeadings(g: Graph, a: Args) {
  const projectUUID = requireString(a, "project_uuid");
  const out: any[] = [];
  for (const t of g.s.tasks.values()) {
    if (t.type === 2 && !t.inTrash && contains(t.parentTaskIDs, projectUUID)) out.push({ uuid: t.uuid, title: t.title });
  }
  return out;
}

function tagList(g: Graph) {
  return [...g.s.tags.values()].map((t) => {
    const o: any = { uuid: t.uuid, title: t.title };
    if (t.shortHand) o.shorthand = t.shortHand;
    if (t.parentTagIDs && t.parentTagIDs.length) o.parentIds = t.parentTagIDs;
    return o;
  });
}

function overview(g: Graph, a: Args) {
  let lookahead = getInt(a, "lookahead_days", 7);
  if (lookahead <= 0) lookahead = 7;
  const s = g.s;
  const areaOrder = [...s.areas.values()].sort((x, y) => (x.title < y.title ? -1 : x.title > y.title ? 1 : 0));
  const areaProjects = new Map<string, Task[]>(areaOrder.map((ar) => [ar.uuid, []]));
  const noArea: Task[] = [];
  for (const t of s.tasks.values()) {
    if (t.type !== 1 || t.status !== 0 || t.inTrash) continue;
    const aid = (t.areaIDs ?? []).find((id) => areaProjects.has(id));
    if (aid) areaProjects.get(aid)!.push(t);
    else noArea.push(t);
  }
  const areas: any[] = areaOrder.map((ar) => ({ uuid: ar.uuid, title: ar.title, projects: sortByIndex(areaProjects.get(ar.uuid)!).map((p) => g.toOutput(p)) }));
  if (noArea.length) areas.push({ uuid: "", title: "(No Area)", projects: sortByIndex(noArea).map((p) => g.toOutput(p)) });

  const end = todayEnd();
  const cutoff = end + lookahead * 86400;
  const { list, projected } = g.candidates();
  const eligible = list.filter((t) => t.type !== 2 && !isTemplate(t) && !isUntitled(t) && g.isEffectivelyActive(t));
  const today = eligible.filter(isScheduledForTodayOrPast);
  const todaySet = new Set(today.map((t) => t.uuid));
  sortByIndex(today);
  const upcoming = eligible.filter((t) => {
    if (todaySet.has(t.uuid)) return false;
    return (t.scheduledDate !== null && t.scheduledDate > end && t.scheduledDate <= cutoff) || (t.deadlineDate !== null && t.deadlineDate > end && t.deadlineDate <= cutoff);
  });
  const upDate = (t: Task) => {
    let best: number | null = t.scheduledDate;
    if (t.deadlineDate !== null && (best === null || t.deadlineDate < best)) best = t.deadlineDate;
    return best ?? -Infinity;
  };
  upcoming.sort((x, y) => upDate(x) - upDate(y));
  return {
    tags: tagList(g),
    areas,
    todayTasks: today.map((t) => g.toOutput(t, projected.has(t))),
    upcomingTasks: upcoming.map((t) => g.toOutput(t, projected.has(t))),
  };
}

function rawWire(t: Task) {
  return {
    uuid: t.uuid, tt: t.title, tp: t.type, ss: t.status, st: t.schedule, ix: t.index, ti: t.todayIndex, do: t.dueOrder,
    sb: t.startBucket, tr: t.inTrash, ar: t.areaIDs, pr: t.parentTaskIDs, agr: t.actionGroupIDs, tg: t.tagIDs,
    rt: t.recurrenceIDs, dl: t.delegateIDs, nt: t.note, cd: t.creationDate ?? -62135596800, md: t.modificationDate,
    sr: t.scheduledDate, dd: t.deadlineDate, sp: t.completionDate, tir: t.todayIndexRefDate, ato: t.alarmTimeOffset, rr: t.repeater,
  };
}

function debugRaw(g: Graph, a: Args) {
  const prefix = requireString(a, "uuid");
  for (const t of g.s.tasks.values()) if (t.uuid.startsWith(prefix)) return rawWire(t);
  throw new ToolError(`task not found: ${prefix}`);
}

// ---------------------------------------------------------------------------
// Write handlers
// ---------------------------------------------------------------------------

const task6 = (uuid: string, t: number, p: any): Envelope => ({ uuid, t, e: "Task6", p });

function collectOpts(a: Args, keys: string[], base: Opts = {}): Opts {
  const opts = { ...base };
  for (const k of keys) {
    const v = getString(a, k);
    if (v) opts[k] = v;
  }
  return opts;
}

function validateRecurrence(opts: Opts) {
  if (opts.recurrence) parseRecurrence(opts.recurrence, nowLocal());
}

function createEnvelopes(uuid: string, payload: any): Envelope[] {
  if (payload.rr) {
    const { templateUUID, template, instance } = splitRecurringPayload(payload);
    return [task6(templateUUID, 0, template), task6(uuid, 0, instance)];
  }
  return [task6(uuid, 0, payload)];
}

async function createTask(ctx: Ctx, g: Graph, a: Args) {
  const title = requireNonEmpty(a, "title");
  const opts = collectOpts(a, ["note", "schedule", "deadline", "project_uuid", "heading_uuid", "area_uuid", "tags", "checklist", "reminder_date", "reminder_time", "recurrence"]);
  g.validateOpts(opts);
  validateRecurrence(opts);
  if (opts.checklist) for (const item of opts.checklist.split(",")) if (item.trim() === "") throw new ToolError("checklist items must not be empty");
  const ix = nextTopIndexBelow(
    g.minIndexWhere((t) => {
      if (t.type !== 0 || t.inTrash || t.status !== 0) return false;
      if (opts.project_uuid && !contains(t.parentTaskIDs, opts.project_uuid)) return false;
      if (opts.heading_uuid && !contains(t.actionGroupIDs, opts.heading_uuid)) return false;
      return true;
    }),
  );
  const uuid = newUUID();
  const envs = createEnvelopes(uuid, newTaskCreatePayload(title, opts, ix));
  if (opts.checklist) {
    const now = nowTs();
    opts.checklist.split(",").forEach((item, i) => {
      envs.push({ uuid: newUUID(), t: 0, e: "ChecklistItem3", p: { cd: now, md: null, tt: item.trim(), ss: 0, sp: null, ix: i, ts: [uuid], lt: false, xx: defaultExtension() } });
    });
  }
  await writeOr(ctx, "create task", envs);
  return { status: "created", uuid, title };
}

async function createProject(ctx: Ctx, g: Graph, a: Args) {
  const title = requireNonEmpty(a, "title");
  const opts = collectOpts(a, ["note", "schedule", "deadline", "area_uuid", "tags", "reminder_date", "reminder_time", "recurrence"], { type: "project" });
  g.validateOpts(opts);
  validateRecurrence(opts);
  const ix = nextTopIndexBelow(g.minIndexWhere((t) => t.type === 1 && !t.inTrash && t.status === 0 && (!opts.area_uuid || contains(t.areaIDs, opts.area_uuid))));
  const uuid = newUUID();
  await writeOr(ctx, "create project", createEnvelopes(uuid, newTaskCreatePayload(title, opts, ix)));
  return { status: "created", uuid, title };
}

async function createHeading(ctx: Ctx, g: Graph, a: Args) {
  const title = requireNonEmpty(a, "title");
  if (typeof a.project_uuid !== "string") throw new ToolError("project_uuid is required");
  const projectUUID: string = a.project_uuid;
  const opts: Opts = { type: "heading", project_uuid: projectUUID };
  g.validateOpts(opts);
  const ix = nextTopIndexBelow(g.minIndexWhere((t) => t.type === 2 && !t.inTrash && t.status === 0 && contains(t.parentTaskIDs, projectUUID)));
  const uuid = newUUID();
  await writeOr(ctx, "create heading", [task6(uuid, 0, newTaskCreatePayload(title, opts, ix))]);
  return { status: "created", uuid, title };
}

async function createArea(ctx: Ctx, _g: Graph, a: Args) {
  const name = requireNonEmpty(a, "name");
  const uuid = newUUID();
  await writeOr(ctx, "create area", [{ uuid, t: 0, e: "Area3", p: { tt: name, ix: 0, tg: [], xx: defaultExtension() } }]);
  return { status: "created", uuid, name };
}

async function createTag(ctx: Ctx, g: Graph, a: Args) {
  const name = requireNonEmpty(a, "name");
  const uuid = newUUID();
  const sh = getString(a, "shorthand") || null;
  let pn: string[] = [];
  const parent = getString(a, "parent_uuid");
  if (parent) {
    g.validateTagParent("", parent);
    pn = [parent];
  }
  await writeOr(ctx, "create tag", [{ uuid, t: 0, e: "Tag4", p: { tt: name, ix: -1237, sh, pn, xx: defaultExtension() } }]);
  return { status: "created", uuid, name };
}

async function editArea(ctx: Ctx, g: Graph, a: Args) {
  const uuid = requireString(a, "uuid");
  g.validateArea(uuid);
  const name = requireNonEmpty(a, "name");
  await writeOr(ctx, "edit area", [{ uuid, t: 1, e: "Area3", p: { tt: name } }]);
  return { status: "updated", uuid };
}

async function deleteArea(ctx: Ctx, g: Graph, a: Args) {
  if (!getBool(a, "confirm")) throw new ToolError("confirm=true is required for permanent area deletion");
  const uuid = requireString(a, "uuid");
  g.validateArea(uuid);
  await writeOr(ctx, "delete area", [{ uuid, t: 2, e: "Area3", p: {} }]);
  return { status: "deleted", uuid };
}

async function editTag(ctx: Ctx, g: Graph, a: Args) {
  const uuid = requireString(a, "uuid");
  g.validateTag(uuid);
  const p: any = {};
  if (getString(a, "name")) p.tt = getString(a, "name");
  if (getString(a, "shorthand")) p.sh = getString(a, "shorthand");
  const parent = getString(a, "parent_uuid");
  if (parent) {
    g.validateTagParent(uuid, parent);
    p.pn = [parent];
  }
  if (Object.keys(p).length === 0) throw new ToolError("at least one field (name, shorthand, parent_uuid) must be provided");
  await writeOr(ctx, "edit tag", [{ uuid, t: 1, e: "Tag4", p }]);
  return { status: "updated", uuid };
}

async function deleteTag(ctx: Ctx, g: Graph, a: Args) {
  if (!getBool(a, "confirm")) throw new ToolError("confirm=true is required for permanent tag deletion");
  const uuid = requireString(a, "uuid");
  g.validateTag(uuid);
  await writeOr(ctx, "delete tag", [{ uuid, t: 2, e: "Tag4", p: {} }]);
  return { status: "deleted", uuid };
}

async function editItem(ctx: Ctx, g: Graph, a: Args) {
  const uuid = requireString(a, "uuid");
  g.validateTask(uuid);
  const target = g.s.tasks.get(uuid) ?? null;
  if (target && isTemplate(target)) {
    throw new ToolError(
      `${uuid} is the projected next occurrence of a recurring to-do (materialized:false); it cannot be completed or edited until Things materializes the real occurrence on next app sync. To change the repeating rule, edit an existing occurrence with the recurrence parameter.`,
    );
  }
  g.validateOpts(collectOpts(a, ["project_uuid", "heading_uuid", "area_uuid", "tags"]));
  const keys = ["title", "note", "schedule", "deadline", "area_uuid", "project_uuid", "heading_uuid", "tags", "reminder_date", "reminder_time", "recurrence", "status"];
  if (!keys.some((k) => k in a)) throw new ToolError("at least one editable field must be provided");

  const u: any = { md: nowTs() };
  const envs: Envelope[] = [];
  const setSchedule = (st: number, sr: number | null, tir: number | null) => Object.assign(u, { st, sr, tir });
  if ("title" in a) {
    const v = getString(a, "title").trim();
    if (!v) throw new ToolError("title must not be empty");
    u.tt = v;
  }
  if ("note" in a) {
    const v = getString(a, "note");
    u.nt = v === "" ? emptyNote() : textNote(v);
  }
  const sched = getString(a, "schedule");
  if (sched) {
    const today = todayMidnightUTC();
    if (sched === "today") { setSchedule(1, today, today); u.sb = 0; }
    else if (sched === "tonight") { setSchedule(1, today, today); u.sb = 1; }
    else if (sched === "anytime") { setSchedule(1, null, null); u.sb = 0; }
    else if (sched === "someday") { setSchedule(2, null, null); u.sb = 0; }
    else if (sched === "inbox") { setSchedule(0, null, null); u.sb = 0; }
    else {
      const dt = parseDate(sched);
      if (!dt) throw new ToolError(`invalid schedule: ${sched}`);
      setSchedule(2, dt.sec, dt.sec);
    }
  }
  const deadline = getString(a, "deadline");
  if (deadline) {
    if (deadline === "none") u.dd = null;
    else {
      const dt = parseDate(deadline);
      if (!dt) throw new ToolError(`invalid deadline: ${deadline}`);
      u.dd = dt.sec;
    }
  }
  const move = (ar: string[], pr: string[], agr: string[]) => {
    Object.assign(u, { ar, pr, agr });
    if (!sched) setSchedule(1, null, null);
  };
  if (getString(a, "area_uuid")) move([getString(a, "area_uuid")], [], []);
  if (getString(a, "project_uuid")) move([], [getString(a, "project_uuid")], []);
  if (getString(a, "heading_uuid")) move([], [], [getString(a, "heading_uuid")]);
  if ("tags" in a) u.tg = splitUUIDs(getString(a, "tags"));
  const rmd = getString(a, "reminder_date");
  if (rmd) {
    if (rmd === "none") Object.assign(u, { rmd: null, ato: null });
    else {
      const dt = parseDate(rmd);
      const off = parseTime(getString(a, "reminder_time"));
      if (!dt || off === null) throw new ToolError("valid reminder_date and reminder_time must be provided together");
      Object.assign(u, { rmd: dt.sec, ato: off });
    }
  } else if ("reminder_time" in a) throw new ToolError("reminder_date and reminder_time must be provided together");

  const rec = getString(a, "recurrence");
  if (rec) {
    if (!target) throw new ToolError(`task not found: ${uuid}`);
    const hasOld = (target.recurrenceIDs?.length ?? 0) > 0;
    const oldUUID = hasOld ? target.recurrenceIDs![0] : "";
    if (hasOld) {
      const old = g.s.tasks.get(oldUUID);
      if (!old || !isTemplate(old)) throw new ToolError(`recurring template not found or invalid: ${oldUUID}`);
    }
    const trashOld = () => {
      if (hasOld) envs.push(task6(oldUUID, 1, { md: nowTs(), tr: true }));
    };
    if (rec === "none") {
      if (!getBool(a, "confirm_destructive")) throw new ToolError("confirm_destructive=true is required to permanently stop recurrence");
      Object.assign(u, { rt: [], rr: null, icsd: null });
      trashOld();
    } else {
      let ref = nowLocal();
      const schedArg = getString(a, "schedule");
      if (schedArg) {
        const dt = parseDate(schedArg);
        if (dt) ref = dt.local;
      } else if (target.todayIndexRefDate !== null) ref = ymd(target.todayIndexRefDate);
      else if (target.scheduledDate !== null) ref = ymd(target.scheduledDate);
      const rr = parseRecurrence(rec, ref);
      if (!rr) throw new ToolError("invalid recurrence");
      const next = nextOccurrenceAfter(rr, ref);
      if (next === null) throw new ToolError("could not compute next recurrence date");
      const newTpl = newUUID();
      envs.push(task6(newTpl, 0, recurringTemplateForEdit(target, a, rr, next)));
      Object.assign(u, { rt: [newTpl], rr: null, icsd: null });
      trashOld();
    }
  }
  const status = getString(a, "status");
  if (status) {
    if (status === "completed") Object.assign(u, { ss: 3, sp: nowTs() });
    else if (status === "canceled") Object.assign(u, { ss: 2, sp: nowTs() });
    else if (status === "pending") Object.assign(u, { ss: 0, sp: null });
    else if (status === "trashed") u.tr = true;
    else if (status === "restored") u.tr = false;
    else throw new ToolError(`invalid status: ${status}`);
  }
  envs.push(task6(uuid, 1, u));
  await writeOr(ctx, "edit task", envs);
  return { status: "updated", uuid };
}

async function addChecklistItem(ctx: Ctx, g: Graph, a: Args) {
  const taskUUID = typeof a.task_uuid === "string" ? a.task_uuid : null;
  if (taskUUID === null) throw new ToolError("task_uuid is required");
  const title = requireNonEmpty(a, "title");
  g.validateTask(taskUUID);
  const uuid = newUUID();
  const p = { cd: nowTs(), md: null, tt: title, ss: 0, sp: null, ix: getInt(a, "index", 0), ts: [taskUUID], lt: false, xx: defaultExtension() };
  await writeOr(ctx, "add checklist item", [{ uuid, t: 0, e: "ChecklistItem3", p }]);
  return { status: "created", uuid, task_uuid: taskUUID };
}

async function editChecklistItem(ctx: Ctx, g: Graph, a: Args) {
  const uuid = requireString(a, "uuid");
  g.validateChecklist(uuid);
  const p: any = { md: nowTs() };
  let changed = false;
  if (getString(a, "title")) { p.tt = getString(a, "title"); changed = true; }
  const ix = getInt(a, "index", -1);
  if (ix >= 0) { p.ix = ix; changed = true; }
  if ("completed" in a) {
    changed = true;
    if (a.completed === true) Object.assign(p, { ss: 3, sp: nowTs() });
    else Object.assign(p, { ss: 0, sp: null });
  }
  if (!changed) throw new ToolError("at least one field (title, index, completed) must be provided");
  await writeOr(ctx, "edit checklist item", [{ uuid, t: 1, e: "ChecklistItem3", p }]);
  return { status: "updated", uuid };
}

async function deleteChecklistItem(ctx: Ctx, g: Graph, a: Args) {
  if (!getBool(a, "confirm")) throw new ToolError("confirm=true is required for permanent checklist item deletion");
  const uuid = requireString(a, "uuid");
  g.validateChecklist(uuid);
  await writeOr(ctx, "delete checklist item", [{ uuid, t: 2, e: "ChecklistItem3", p: {} }]);
  return { status: "deleted", uuid };
}

async function writeOr(ctx: Ctx, what: string, envs: Envelope[]) {
  try {
    await ctx.write(envs);
  } catch (e: any) {
    throw new ToolError(`${what}: ${e?.message ?? e}`);
  }
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

type Handler = (ctx: Ctx, g: Graph, a: Args) => any;

export const HANDLERS: Record<string, Handler> = {
  things_find_tasks: (_c, g, a) => findTasks(g, a),
  things_show_task: (_c, g, a) => showTask(g, a),
  things_show_project: (_c, g, a) => showProject(g, a),
  things_find_projects: (_c, g, a) => findProjects(g, a),
  things_find_headings: (_c, g, a) => findHeadings(g, a),
  things_find_areas: (_c, g) => [...g.s.areas.values()].map((ar) => ({ uuid: ar.uuid, title: ar.title })),
  things_find_tags: (_c, g) => tagList(g),
  things_overview: (_c, g, a) => overview(g, a),
  things_create_task: createTask,
  things_create_heading: createHeading,
  things_create_project: createProject,
  things_create_area: createArea,
  things_create_tag: createTag,
  things_edit_area: editArea,
  things_delete_area: deleteArea,
  things_edit_tag: editTag,
  things_delete_tag: deleteTag,
  things_edit_item: editItem,
  things_add_checklist_item: addChecklistItem,
  things_edit_checklist_item: editChecklistItem,
  things_delete_checklist_item: deleteChecklistItem,
  things_debug_raw: (_c, g, a) => debugRaw(g, a),
};

export async function runTool(ctx: Ctx, name: string, args: Args): Promise<any> {
  const h = HANDLERS[name];
  if (!h) throw new ToolError(`tool not found: ${name}`);
  return h(ctx, new Graph(ctx.state), args ?? {});
}

// Used by diagnose (step 7).
export function activeCounts(s: State): { activeTasks: number; activeProjects: number } {
  const g = new Graph(s);
  let activeTasks = 0, activeProjects = 0;
  for (const t of s.tasks.values()) {
    if (t.inTrash) continue;
    if (t.type === 0 && g.isEffectivelyActive(t)) activeTasks++;
    if (t.type === 1 && t.status === 0) activeProjects++;
  }
  return { activeTasks, activeProjects };
}

export type { RawItem };
