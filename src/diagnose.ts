// things_diagnose: a read-only walk through the whole sync pipeline with a
// fresh client, reported step by step.

import { State, ThingsClient, Cursor, RawItem, ymd } from "./things";

interface Step {
  step: number;
  name: string;
  description: string;
  status: string;
  durationMs: number;
  details: any;
  log: string[];
}

const STEP_DEFS: [number, string, string][] = [
  [1, "credential_verification", "Verify Things Cloud credentials and account status"],
  [2, "fetch_history", "Resolve the authoritative account history and inspect other keys"],
  [3, "sync_history", "Sync the authoritative history to get latest server index"],
  [4, "paginated_fetch", "Fetch all items via paginated API"],
  [5, "rebuild_state", "Rebuild in-memory state from items"],
  [6, "data_integrity", "Check data completeness and integrity"],
  [7, "query_tests", "Test basic list/query operations"],
];

export function maskEmail(email: string): string {
  const i = email.indexOf("@");
  if (i <= 0) return "***";
  return email[0] + "***@" + email.slice(i + 1);
}

const msg = (e: any) => String(e?.message ?? e);
const day = (sec: number) => new Date(Math.floor(sec * 1000)).toISOString().slice(0, 10);

export async function diagnose(
  email: string,
  password: string,
  endpoint: string,
  syncMain: () => Promise<{ activeTasks: number; activeProjects: number }>,
  mainCounts: () => { areas: number; tags: number },
) {
  const steps: Step[] = [];
  const warnings: string[] = [];
  const errors: string[] = [];
  const finish = () => {
    const summary = { totalSteps: steps.length, passed: 0, warnings: 0, failed: 0, skipped: 0, totalDurationMs: 0 };
    for (const s of steps) {
      summary.totalDurationMs += s.durationMs;
      if (s.status === "pass") summary.passed++;
      else if (s.status === "warn") summary.warnings++;
      else if (s.status === "fail") summary.failed++;
      else if (s.status === "skipped") summary.skipped++;
    }
    return { steps, summary, warnings, errors };
  };
  const skipFrom = (n: number) => {
    for (const [num, name, description] of STEP_DEFS) {
      if (num >= n) steps.push({ step: num, name, description, status: "skipped", durationMs: 0, details: { reason: "previous step failed" }, log: [] });
    }
  };
  const newStep = (n: number, description?: string): Step => {
    const def = STEP_DEFS[n - 1];
    return { step: n, name: def[1], description: description ?? def[2], status: "", durationMs: 0, details: null, log: [] };
  };

  // Step 1
  const s1 = newStep(1);
  s1.log.push(`Verifying credentials for ${maskEmail(email)}`);
  let t0 = Date.now();
  const client = new ThingsClient(email, password, endpoint);
  let verified;
  try {
    verified = await client.verify();
  } catch (e) {
    s1.durationMs = Date.now() - t0;
    s1.status = "fail";
    s1.details = { error: msg(e) };
    s1.log.push(`Credential verification failed: ${msg(e)}`);
    errors.push(`Step 1: credential verification failed: ${msg(e)}`);
    steps.push(s1);
    skipFrom(2);
    return finish();
  }
  s1.durationMs = Date.now() - t0;
  s1.status = "pass";
  s1.details = { accountStatus: verified.status, historyKey: verified.historyKey, email: maskEmail(verified.email) };
  s1.log.push(`Account status: ${verified.status}`, `History key from /verify: ${verified.historyKey}`);
  steps.push(s1);

  // Step 2
  const s2 = newStep(2);
  t0 = Date.now();
  const ownID = verified.historyKey;
  let history: Cursor;
  try {
    const own = await client.verify();
    history = await client.history(own.historyKey).catch((e) => {
      throw new Error(`fetch authoritative history ${own.historyKey}: ${msg(e)}`);
    });
  } catch (e) {
    s2.durationMs = Date.now() - t0;
    s2.status = "fail";
    s2.details = { error: msg(e) };
    s2.log.push(`bestHistory failed: ${msg(e)}`);
    errors.push(`Step 2: bestHistory failed: ${msg(e)}`);
    steps.push(s2);
    skipFrom(3);
    return finish();
  }
  const all: any[] = [];
  let keys: string[] = [];
  let keysErr: any = null;
  try {
    keys = await client.ownHistoryKeys();
  } catch (e) {
    keysErr = e;
    const w = `Histories() returned error: ${msg(e)}`;
    s2.log.push(`Warning: ${w}`);
    warnings.push(`Step 2: ${w}`);
  }
  if (!keysErr && keys.length === 0) {
    const w = "own-history-keys returned 0 keys; falling back to account history key from /verify";
    s2.log.push(`Warning: ${w}`);
    warnings.push(`Step 2: ${w}`);
  }
  let ownFound = false;
  for (const id of keys) {
    let full: Cursor;
    try {
      full = await client.history(id);
    } catch (e) {
      s2.log.push(`History ${id}: failed to fetch metadata: ${msg(e)}`);
      continue;
    }
    const hi: any = { id: full.id, latestServerIndex: full.latestServerIndex, isOwnHistory: full.id === ownID, selected: full.id === history.id };
    if (hi.isOwnHistory) ownFound = true;
    if (full.latestServerIndex > 0) {
      try {
        const { items } = await client.items(full, Math.max(0, full.latestServerIndex - 5));
        let newest = -Infinity;
        for (const it of items) {
          if (!/^Task\d*$/.test(it.e) || !["Task6", "Task7", "Task4", "Task3", "Task"].includes(it.e)) continue;
          const cd = it.p?.cd;
          if (typeof cd === "number" && cd > newest) newest = cd;
        }
        if (newest > -Infinity) hi.newestItemDate = day(newest);
      } catch (e) {
        s2.log.push(`History ${full.id}: failed to peek items: ${msg(e)}`);
      }
    }
    all.push(hi);
    s2.log.push(`History ${full.id}: serverIndex=${hi.latestServerIndex}, isOwnHistory=${hi.isOwnHistory}, selected=${hi.selected}` + (hi.newestItemDate ? `, newestItemDate=${hi.newestItemDate}` : ""));
  }
  if (keys.length > 0 && !ownFound) {
    const w = `Account history key (${ownID}) not found in own-history-keys list`;
    s2.log.push(`Warning: ${w}`);
    warnings.push(`Step 2: ${w}`);
  }
  let selectedIndex = 0;
  if (keys.length === 0) {
    try {
      const meta = await client.history(history.id);
      selectedIndex = meta.latestServerIndex;
      s2.log.push(`Selected history metadata: serverIndex=${meta.latestServerIndex}`);
    } catch (e) {
      s2.log.push(`Selected history metadata fetch failed: ${msg(e)}`);
    }
  } else selectedIndex = history.latestServerIndex;
  s2.durationMs = Date.now() - t0;
  s2.status = "pass";
  s2.details = { historyCount: keys.length, selectedHistory: history.id, selectedServerIndexMeta: selectedIndex, ownHistoryKey: ownID, selectedIsSameAsOwn: history.id === ownID, allHistories: all.length ? all : null };
  if (history.id !== ownID && keys.length > 1) {
    s2.log.push(`WARNING: Selected history ${history.id} differs from OwnHistory ${ownID} — account may have multiple sync streams`);
    warnings.push("Selected history differs from OwnHistory — multi-device account detected");
  }
  s2.log.push(`Selected history: ${history.id} (serverIndex=${history.latestServerIndex})`);
  steps.push(s2);

  // Step 3: History.Sync — re-read the head index.
  const s3 = newStep(3);
  const pre = history.latestServerIndex;
  t0 = Date.now();
  try {
    const probe: Cursor = { ...history };
    await client.items(probe, history.latestServerIndex).catch(async (e) => {
      throw e;
    });
    history.latestServerIndex = probe.latestServerIndex;
    history.latestSchemaVersion = probe.latestSchemaVersion;
  } catch (e) {
    s3.durationMs = Date.now() - t0;
    s3.status = "fail";
    s3.details = { error: msg(e) };
    s3.log.push(`History sync failed: ${msg(e)}`);
    errors.push(`Step 3: history sync failed: ${msg(e)}`);
    steps.push(s3);
    skipFrom(4);
    return finish();
  }
  s3.durationMs = Date.now() - t0;
  s3.status = "pass";
  s3.details = { preSyncServerIndex: pre, postSyncServerIndex: history.latestServerIndex };
  s3.log.push(`Server index: ${pre} → ${history.latestServerIndex} (after sync)`);
  steps.push(s3);

  // Step 4
  const s4 = newStep(4);
  const items: RawItem[] = [];
  const pages: any[] = [];
  let start = 0, pageNum = 0;
  t0 = Date.now();
  for (;;) {
    pageNum++;
    let page;
    try {
      page = await client.items(history, start);
    } catch (e) {
      s4.durationMs = Date.now() - t0;
      s4.status = "fail";
      s4.details = { error: msg(e), totalItemsFetched: items.length, paginationPages: pageNum, pages: pages.length ? pages : null, finalServerIndex: history.latestServerIndex };
      s4.log.push(`Page ${pageNum} failed at startIndex ${start}: ${msg(e)}`);
      errors.push(`Step 4: paginated fetch failed on page ${pageNum}: ${msg(e)}`);
      steps.push(s4);
      skipFrom(5);
      return finish();
    }
    if (page.items.length === 0) break;
    items.push(...page.items);
    pages.push({ page: pageNum, startIndex: start, itemsFetched: page.items.length, serverIndexAfter: history.latestServerIndex });
    s4.log.push(`Page ${pageNum}: startIndex=${start}, fetched=${page.items.length}, serverIndex=${history.latestServerIndex}`);
    if (!page.hasMore) break;
    start = history.loadedServerIndex;
  }
  s4.durationMs = Date.now() - t0;
  s4.status = "pass";
  const kinds: Record<string, number> = {};
  for (const it of items) kinds[it.e] = (kinds[it.e] ?? 0) + 1;
  const tail = items.slice(Math.max(0, items.length - 5)).map((it) => {
    const ti: any = { kind: it.e, action: it.t };
    if (["Task6", "Task7", "Task4", "Task3", "Task"].includes(it.e) && it.p && typeof it.p === "object") {
      if (typeof it.p.cd === "number") ti.creationDate = new Date(Math.floor(it.p.cd) * 1000).toISOString().slice(0, 19) + "Z";
      if (typeof it.p.tt === "string") {
        const bytes = new TextEncoder().encode(it.p.tt);
        ti.title = bytes.length > 30 ? new TextDecoder().decode(bytes.slice(0, 30)) + "..." : it.p.tt;
      }
    }
    return ti;
  });
  s4.details = { totalItemsFetched: items.length, paginationPages: pages.length, pages: pages.length ? pages : null, finalServerIndex: pages.length ? pages[pages.length - 1].serverIndexAfter : 0, itemTypes: kinds, tailItems: tail.length ? tail : null };
  s4.log.push(`Total items fetched: ${items.length} across ${pages.length} pages`);
  s4.log.push(`Item types: map[${Object.keys(kinds).sort().map((k) => `${k}:${kinds[k]}`).join(" ")}]`);
  for (const ti of tail) {
    let desc = ti.kind;
    if (ti.creationDate) desc += " created=" + ti.creationDate;
    if (ti.title) desc += " title=" + ti.title;
    s4.log.push(`Tail item: ${desc} (action=${ti.action})`);
  }
  steps.push(s4);

  // Step 5
  const s5 = newStep(5);
  t0 = Date.now();
  const state = new State();
  try {
    state.update(items);
  } catch (e) {
    s5.durationMs = Date.now() - t0;
    s5.status = "fail";
    s5.details = { error: msg(e) };
    s5.log.push(`State rebuild failed: ${msg(e)}`);
    errors.push(`Step 5: state rebuild failed: ${msg(e)}`);
    steps.push(s5);
    skipFrom(6);
    return finish();
  }
  s5.durationMs = Date.now() - t0;
  s5.status = "pass";
  s5.details = { tasks: state.tasks.size, areas: state.areas.size, tags: state.tags.size, checklistItems: state.checklist.size };
  s5.log.push(`State rebuilt: ${state.tasks.size} tasks, ${state.areas.size} areas, ${state.tags.size} tags, ${state.checklist.size} checklist items`);
  steps.push(s5);

  // Step 6
  const s6 = newStep(6, "Check data integrity and referential consistency");
  t0 = Date.now();
  let active = 0, completed = 0, canceled = 0, trashed = 0;
  const years: Record<number, number> = {};
  let oldest = Infinity, newest = -Infinity;
  for (const t of state.tasks.values()) {
    if (t.type !== 0) continue;
    if (t.inTrash) trashed++;
    else if (t.status === 3) completed++;
    else if (t.status === 2) canceled++;
    else active++;
    if (t.creationDate === null || ymd(t.creationDate).y < 2000) continue;
    const y = ymd(t.creationDate).y;
    years[y] = (years[y] ?? 0) + 1;
    oldest = Math.min(oldest, t.creationDate);
    newest = Math.max(newest, t.creationDate);
  }
  const yearList = Object.keys(years).map(Number).sort((a, b) => a - b);
  s6.log.push(`Task status counts — active=${active}, completed=${completed}, canceled=${canceled}, trashed=${trashed}`);
  const any = newest > -Infinity;
  if (any) s6.log.push(`Oldest task: ${day(oldest)}`, `Newest task: ${day(newest)}`);
  s6.log.push(`Year distribution: ${yearList.map((y) => `${y}=${years[y]}`).join(", ")}`);
  const stepWarnings: string[] = [];
  let daysSince = 0;
  if (any) {
    daysSince = Math.trunc((Date.now() / 1000 - newest) / 86400);
    s6.log.push(`Days since newest task: ${daysSince}`);
    if (daysSince > 3) {
      const w = `Newest task created ${daysSince} days ago (${day(newest)})`;
      stepWarnings.push(w);
      s6.log.push(`Warning: ${w}`);
    }
  }
  for (let i = 1; i < yearList.length; i++) {
    if (yearList[i] - yearList[i - 1] > 1) {
      const a = yearList[i - 1] + 1, b = yearList[i] - 1;
      const w = a === b ? `Gap detected: no tasks in year ${a}` : `Gap detected: no tasks in years ${a}–${b}`;
      stepWarnings.push(w);
      s6.log.push(`Warning: ${w}`);
    }
  }
  s6.durationMs = Date.now() - t0;
  const d6: any = { active, completed, canceled, trashed, yearDistribution: years };
  if (any) Object.assign(d6, { oldestTask: day(oldest), newestTask: day(newest), daysSinceNewest: daysSince });
  if (stepWarnings.length) d6.warnings = stepWarnings;
  s6.details = d6;
  s6.status = stepWarnings.length ? "warn" : "pass";
  for (const w of stepWarnings) warnings.push(`Step 6: ${w}`);
  steps.push(s6);

  // Step 7: the account's own graph
  const s7 = newStep(7, "Run sample queries against rebuilt state");
  t0 = Date.now();
  const results: any[] = [];
  let syncErr: any = null;
  let counts = { activeTasks: 0, activeProjects: 0 };
  try {
    counts = await syncMain();
    s7.log.push("syncAndRebuild succeeded");
    results.push({ name: "syncAndRebuild", ok: true, count: 0 });
  } catch (e) {
    syncErr = e;
    s7.log.push(`syncAndRebuild failed: ${msg(e)}`);
    results.push({ name: "syncAndRebuild", ok: false, count: 0, error: msg(e) });
  }
  const mc = mainCounts();
  results.push({ name: "activeTasks", ok: true, count: counts.activeTasks }, { name: "activeProjects", ok: true, count: counts.activeProjects });
  s7.log.push(`Active tasks: ${counts.activeTasks}`, `Active projects: ${counts.activeProjects}`);
  results.push({ name: "areas", ok: true, count: mc.areas }, { name: "tags", ok: true, count: mc.tags });
  s7.log.push(`Areas: ${mc.areas}`, `Tags: ${mc.tags}`);
  s7.durationMs = Date.now() - t0;
  s7.details = { queryResults: results };
  if (syncErr) {
    s7.status = "fail";
    errors.push(`Step 7: syncAndRebuild failed: ${msg(syncErr)}`);
  } else s7.status = "pass";
  steps.push(s7);
  return finish();
}
