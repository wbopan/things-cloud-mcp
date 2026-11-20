// End-to-end tests of the sync / write lifecycle against a fake Things Cloud.
import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, Server } from "node:http";
import { Session, Store, Meta } from "../src/session";
import { State } from "../src/things";

class Fake {
  hid = "test-history";
  conflictNextCommit = false;
  throttleNextRead = false;
  reset(hid: string, maps: Record<string, any>[]) {
    this.hid = hid;
    this.maps = maps;
  }
  maps: Record<string, any>[] = [];
  commits: any[] = [];
  current: number | null = null; // overrides current-item-index
  failNextCommit = false;
  server!: Server;
  url = "";
  async start() {
    this.server = createServer((req, res) => {
      const u = new URL(req.url!, "http://x");
      const send = (code: number, body: any) => {
        res.writeHead(code, { "Content-Type": "application/json" });
        res.end(JSON.stringify(body));
      };
      if (u.pathname.startsWith("/version/1/account/")) {
        if (u.pathname.endsWith("/own-history-keys")) return send(200, [this.hid]);
        if (req.headers.authorization !== "Password pw") return send(401, {});
        return send(200, { status: "SYAccountStatusActive", "history-key": this.hid, email: "a@b.c" });
      }
      if (u.pathname.startsWith("/version/1/history/") && !u.pathname.startsWith(`/version/1/history/${this.hid}`)) return send(404, {});
      if (u.pathname === `/version/1/history/${this.hid}`) return send(200, { "latest-server-index": this.maps.length, "latest-schema-version": 301 });
      if (u.pathname === `/version/1/history/${this.hid}/items`) {
        if (this.throttleNextRead) {
          this.throttleNextRead = false;
          res.writeHead(429, { "Retry-After": "1" });
          return res.end("{}");
        }
        const s = Number(u.searchParams.get("start-index"));
        return send(200, { items: this.maps.slice(s, s + 2), "current-item-index": this.current ?? this.maps.length, schema: 301 });
      }
      if (u.pathname === `/version/1/history/${this.hid}/commit` && req.method === "POST") {
        if (this.conflictNextCommit) {
          this.conflictNextCommit = false;
          this.maps.push({ [TASK]: { t: 1, e: "Task6", p: { md: now } } }); // the other device's commit
          res.writeHead(409, { "Things-Response": "OutdatedAncestor" });
          return res.end("{}");
        }
        let body = "";
        req.on("data", (c) => (body += c));
        req.on("end", () => {
          const b = JSON.parse(body);
          this.commits.push(b);
          this.maps.push(b);
          if (this.failNextCommit) {
            this.failNextCommit = false;
            res.destroy();
            return;
          }
          send(200, { "server-head-index": this.maps.length });
        });
        return;
      }
      send(404, {});
    });
    await new Promise<void>((r) => this.server.listen(0, "127.0.0.1", r));
    this.url = `http://127.0.0.1:${(this.server.address() as any).port}`;
  }
  stop() {
    this.server.close();
  }
}

class MemStore implements Store {
  meta: Meta | null = null;
  rows = new Map<string, string>();
  loadMeta() {
    return this.meta && JSON.parse(JSON.stringify(this.meta));
  }
  loadState() {
    const s = new State();
    for (const [k, v] of this.rows) {
      const [kind, uuid] = k.split("|");
      s.mapFor(kind as any).set(uuid, JSON.parse(v));
    }
    return s;
  }
  save(meta: Meta, state: State, changed: { kind: any; uuid: string }[]) {
    this.meta = JSON.parse(JSON.stringify(meta));
    for (const { kind, uuid } of changed) {
      const e = state.get(kind, uuid);
      if (e) this.rows.set(`${kind}|${uuid}`, JSON.stringify(e));
      else this.rows.delete(`${kind}|${uuid}`);
    }
  }
  replaceAll(meta: Meta, state: State) {
    this.rows.clear();
    const all = (["task", "area", "tag", "checklist"] as const).flatMap((kind) => [...state.mapFor(kind).keys()].map((uuid) => ({ kind, uuid })));
    this.save(meta, state, all);
  }
}

const AREA = "5Wh9sPuMRdTL8rpYD8vyJn";
const TASK = "7Qv2TRwMVQcbUH4xtZk3qS";
const now = Date.now() / 1000;
const task = (title: string, extra: any = {}) => ({ tt: title, tp: 0, ss: 0, st: 1, ix: 0, cd: now - 100, md: now - 100, tr: false, ar: [], pr: [], agr: [], tg: [], ...extra });

async function setup() {
  const fake = new Fake();
  await fake.start();
  fake.maps.push({ [AREA]: { t: 0, e: "Area3", p: { tt: "Work", ix: 0 } } });
  fake.maps.push({ [TASK]: { t: 0, e: "Task6", p: task("Write report", { ar: [AREA] }) } });
  fake.maps.push({ S1: { t: 1, e: "Settings5", p: { anything: true } } });
  const store = new MemStore();
  return { fake, store, session: new Session(store, fake.url) };
}

test("full rebuild, read, write and reload from the store", async () => {
  const { fake, store, session } = await setup();
  try {
    const found = await session.callTool("a@b.c", "pw", "things_find_tasks", { area: "work" });
    assert.equal(found.error, undefined);
    assert.deepEqual(found.data.map((t: any) => t.title), ["Write report"]);

    const created = await session.callTool("a@b.c", "pw", "things_create_task", { title: "New one", schedule: "today", area_uuid: AREA });
    assert.equal(created.error, undefined, created.error);
    assert.equal(fake.commits.length, 1);
    const [uuid, env] = Object.entries<any>(fake.commits[0])[0];
    assert.equal(env.e, "Task6");
    assert.equal(env.p.tt, "New one");
    assert.deepEqual(env.p.ar, [AREA]);

    // A fresh session (an evicted Durable Object) serves from the store.
    const reloaded = new Session(store, fake.url);
    const shown = await reloaded.callTool("a@b.c", "pw", "things_show_task", { uuid });
    assert.equal(shown.data.title, "New one");
  } finally {
    fake.stop();
  }
});

test("a wrong password never reads the cached graph", async () => {
  const { fake, session } = await setup();
  try {
    assert.equal((await session.callTool("a@b.c", "pw", "things_find_areas", {})).error, undefined);
    const r = await session.callTool("a@b.c", "wrong", "things_find_areas", {});
    assert.match(r.error!, /^login: /);
  } finally {
    fake.stop();
  }
});

test("incremental sync picks up changes from other devices", async () => {
  const { fake, session } = await setup();
  try {
    await session.callTool("a@b.c", "pw", "things_find_tasks", {});
    fake.maps.push({ [TASK]: { t: 1, e: "Task6", p: { tt: "Renamed on the Mac", md: now } } });
    await new Promise((r) => setTimeout(r, 2100));
    const r = await session.callTool("a@b.c", "pw", "things_show_task", { uuid: TASK });
    assert.equal(r.data.title, "Renamed on the Mac");
  } finally {
    fake.stop();
  }
});

test("sync fails closed on a cursor regression and on unknown kinds", async () => {
  const { fake, session } = await setup();
  try {
    await session.callTool("a@b.c", "pw", "things_find_tasks", {});
    fake.maps.push({ X: { t: 0, e: "Mystery9", p: {} } });
    await new Promise((r) => setTimeout(r, 2100));
    const r = await session.callTool("a@b.c", "pw", "things_find_tasks", {});
    assert.match(r.error!, /^sync: .*unsupported kind "Mystery9"/);
    fake.maps.pop();
    fake.current = 1;
    const r2 = await session.callTool("a@b.c", "pw", "things_find_tasks", {});
    assert.match(r2.error!, /^sync: .*regressed/);
  } finally {
    fake.stop();
  }
});

test("an uncertain commit is reconciled from history, never retried", async () => {
  const { fake, session } = await setup();
  try {
    await session.callTool("a@b.c", "pw", "things_find_tasks", {});
    fake.failNextCommit = true;
    const r = await session.callTool("a@b.c", "pw", "things_edit_item", { uuid: TASK, title: "Edited" });
    assert.equal(r.error, undefined, r.error);
    assert.equal(fake.commits.length, 1);
    const shown = await session.callTool("a@b.c", "pw", "things_show_task", { uuid: TASK });
    assert.equal(shown.data.title, "Edited");
  } finally {
    fake.stop();
  }
});

test("destructive operations need confirmation", async () => {
  const { fake, session } = await setup();
  try {
    const r = await session.callTool("a@b.c", "pw", "things_delete_area", { uuid: AREA, confirm: false });
    assert.ok(r.error);
    assert.equal(fake.commits.length, 0);
  } finally {
    fake.stop();
  }
});

test("Mail to Things queue records and legacy Task2/Tag2 kinds load", async () => {
  const { fake, session } = await setup();
  try {
    fake.maps.push({ "8F63FEC2-1202-43D8-80D7-55B4AC4FCA37": { t: 0, e: "Command", p: { cd: now, tp: 0, if: { tt: "Fwd: hello", nt: "<note>body</note>" } } } });
    fake.maps.push({ "457jbco4TLEAXyjgnrgE6X": { t: 2, e: "Command3", p: {} } });
    fake.maps.push({ "legacy-task-id": { t: 0, e: "Task2", p: task("Old task") }, "legacy-tag-id": { t: 0, e: "Tag2", p: { tt: "oldtag", ix: 0 } } });
    const r = await session.callTool("a@b.c", "pw", "things_find_tasks", {});
    assert.equal(r.error, undefined, r.error);
    assert.deepEqual(r.data.map((t: any) => t.title).sort(), ["Old task", "Write report"]);
    const tags = await session.callTool("a@b.c", "pw", "things_find_tags", {});
    assert.deepEqual(tags.data.map((t: any) => t.title), ["oldtag"]);
    fake.maps.push({ X: { t: 0, e: "Command4", p: {} } });
    await new Promise((r) => setTimeout(r, 2100));
    assert.match((await session.callTool("a@b.c", "pw", "things_find_tasks", {})).error!, /unsupported kind "Command4"/);
  } finally {
    fake.stop();
  }
});

test("a task with no title but a note is listed", async () => {
  const { fake, session } = await setup();
  try {
    fake.maps.push({ "4WfT7Xc9RdWkqhYbm5fQmA": { t: 0, e: "Task6", p: task("", { nt: { _t: "tx", t: 1, ch: 0, v: "https://example.com" } }) } });
    fake.maps.push({ "4WfT7Xc9RdWkqhYbm5fQmB": { t: 0, e: "Task6", p: task("") } });
    const r = await session.callTool("a@b.c", "pw", "things_find_tasks", {});
    assert.equal(r.data.length, 2);
  } finally {
    fake.stop();
  }
});

test("409 OutdatedAncestor resyncs and resends; 429 on reads backs off", async () => {
  const { fake, session } = await setup();
  try {
    await session.callTool("a@b.c", "pw", "things_find_tasks", {});
    fake.conflictNextCommit = true;
    const r = await session.callTool("a@b.c", "pw", "things_edit_item", { uuid: TASK, title: "After conflict" });
    assert.equal(r.error, undefined, r.error);
    assert.equal(fake.commits.length, 1);
    fake.throttleNextRead = true;
    await new Promise((r) => setTimeout(r, 2100));
    const shown = await session.callTool("a@b.c", "pw", "things_show_task", { uuid: TASK });
    assert.equal(shown.data.title, "After conflict");
  } finally {
    fake.stop();
  }
});

test("an account reset (new history) is rebuilt instead of failing forever", async () => {
  const { fake, session } = await setup();
  try {
    await session.callTool("a@b.c", "pw", "things_find_tasks", {});
    fake.reset("new-history", [{ [TASK]: { t: 0, e: "Task6", p: task("Recreated") } }]);
    await new Promise((r) => setTimeout(r, 2100));
    const r = await session.callTool("a@b.c", "pw", "things_find_tasks", {});
    assert.equal(r.error, undefined, r.error);
    assert.deepEqual(r.data.map((t: any) => t.title), ["Recreated"]);
  } finally {
    fake.stop();
  }
});
