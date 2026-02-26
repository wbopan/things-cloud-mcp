package main

import "net/http"

// LandingPageHTML contains the full HTML landing page styled after Things 3's design language.
const LandingPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Things Cloud MCP</title>
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}

:root{
  --blue:#1A7CF9;
  --blue-hover:#0A6CE9;
  --bg:#FFFFFF;
  --bg-alt:#F7F7F7;
  --text:#1C1C1E;
  --text-secondary:#8E8E93;
  --green:#34C759;
  --surface:#FFFFFF;
  --shadow:0 1px 4px rgba(0,0,0,0.08);
  --shadow-lg:0 4px 16px rgba(0,0,0,0.08);
  --divider:#E5E5EA;
  --code-bg:#F2F2F7;
  --radius:12px;
  --radius-sm:8px;
}

@media(prefers-color-scheme:dark){
  :root{
    --bg:#1C1C1E;
    --bg-alt:#2C2C2E;
    --text:#F2F2F7;
    --text-secondary:#8E8E93;
    --surface:#2C2C2E;
    --shadow:0 1px 4px rgba(0,0,0,0.3);
    --shadow-lg:0 4px 16px rgba(0,0,0,0.3);
    --divider:#3A3A3C;
    --code-bg:#3A3A3C;
  }
}

html{scroll-behavior:smooth}

body{
  font-family:-apple-system,BlinkMacSystemFont,"SF Pro Display","SF Pro Text","Segoe UI",Roboto,Helvetica,Arial,sans-serif;
  color:var(--text);
  background:var(--bg);
  line-height:1.6;
  font-size:16px;
  -webkit-font-smoothing:antialiased;
  -moz-osx-font-smoothing:grayscale;
}

.container{max-width:960px;margin:0 auto;padding:0 24px}

/* ── Header ── */
header{
  background:var(--bg);
  border-bottom:1px solid var(--divider);
  padding:20px 0;
  position:sticky;
  top:0;
  z-index:100;
  backdrop-filter:blur(20px);
  -webkit-backdrop-filter:blur(20px);
  background:rgba(255,255,255,0.85);
}
@media(prefers-color-scheme:dark){
  header{background:rgba(28,28,30,0.85)}
}
.header-inner{
  display:flex;
  align-items:center;
  gap:14px;
}
.logo{
  width:40px;height:40px;
  background:var(--blue);
  border-radius:10px;
  display:flex;align-items:center;justify-content:center;
  flex-shrink:0;
}
.logo .checkmark{
  width:18px;height:18px;
  position:relative;
}
.logo .checkmark::after{
  content:"";
  position:absolute;
  left:3px;top:1px;
  width:6px;height:12px;
  border:solid #fff;
  border-width:0 2.5px 2.5px 0;
  transform:rotate(45deg);
}
.header-text h1{
  font-size:20px;
  font-weight:700;
  letter-spacing:-0.3px;
  color:var(--text);
}
.header-text p{
  font-size:13px;
  color:var(--text-secondary);
  font-weight:400;
  margin-top:-1px;
}

/* ── Hero ── */
.hero{
  padding:80px 0 64px;
  text-align:center;
}
.hero-icon{
  width:80px;height:80px;
  background:var(--blue);
  border-radius:20px;
  display:inline-flex;align-items:center;justify-content:center;
  margin-bottom:28px;
  box-shadow:0 8px 24px rgba(26,124,249,0.25);
}
.hero-icon .checkmark-lg{
  position:relative;
  width:36px;height:36px;
}
.hero-icon .checkmark-lg::after{
  content:"";
  position:absolute;
  left:7px;top:3px;
  width:13px;height:25px;
  border:solid #fff;
  border-width:0 4px 4px 0;
  transform:rotate(45deg);
}
.hero h2{
  font-size:40px;
  font-weight:700;
  letter-spacing:-0.8px;
  margin-bottom:16px;
  line-height:1.15;
}
.hero h2 .gradient{
  background:linear-gradient(135deg,var(--blue),#5856D6);
  -webkit-background-clip:text;
  -webkit-text-fill-color:transparent;
  background-clip:text;
}
.hero p{
  font-size:18px;
  color:var(--text-secondary);
  max-width:540px;
  margin:0 auto 32px;
  line-height:1.55;
}
.hero-meta{
  display:inline-flex;
  gap:24px;
  flex-wrap:wrap;
  justify-content:center;
}
.hero-meta .badge{
  display:inline-flex;
  align-items:center;
  gap:6px;
  background:var(--bg-alt);
  padding:8px 16px;
  border-radius:20px;
  font-size:13px;
  font-weight:500;
  color:var(--text-secondary);
}
.hero-meta .badge .dot{
  width:7px;height:7px;
  border-radius:50%;
  background:var(--green);
  flex-shrink:0;
}

/* ── Section shared ── */
section{padding:64px 0}
section.alt{background:var(--bg-alt)}
.section-label{
  font-size:13px;
  font-weight:600;
  text-transform:uppercase;
  letter-spacing:0.8px;
  color:var(--blue);
  margin-bottom:8px;
}
.section-title{
  font-size:28px;
  font-weight:700;
  letter-spacing:-0.5px;
  margin-bottom:8px;
}
.section-desc{
  color:var(--text-secondary);
  font-size:15px;
  margin-bottom:36px;
  max-width:560px;
}
.section-header{margin-bottom:36px}

/* ── Tools Grid ── */
.tools-category{margin-bottom:40px}
.tools-category:last-child{margin-bottom:0}
.category-label{
  display:inline-flex;
  align-items:center;
  gap:8px;
  font-size:12px;
  font-weight:600;
  text-transform:uppercase;
  letter-spacing:0.8px;
  color:var(--text-secondary);
  margin-bottom:16px;
}
.category-dot{
  width:8px;height:8px;
  border-radius:50%;
  flex-shrink:0;
}
.category-dot.read{background:#007AFF}
.category-dot.create{background:#34C759}
.category-dot.modify{background:#FF9500}
.category-dot.advanced{background:#AF52DE}

.tools-grid{
  display:grid;
  grid-template-columns:repeat(auto-fill,minmax(260px,1fr));
  gap:12px;
}
.tool-card{
  background:var(--surface);
  border-radius:var(--radius);
  padding:18px 20px;
  box-shadow:var(--shadow);
  transition:box-shadow 0.2s ease,transform 0.2s ease;
  cursor:default;
}
.tool-card:hover{
  box-shadow:var(--shadow-lg);
  transform:translateY(-1px);
}
.tool-name{
  font-size:14px;
  font-weight:600;
  font-family:"SF Mono",SFMono-Regular,Menlo,Monaco,Consolas,monospace;
  color:var(--text);
  margin-bottom:4px;
  display:flex;
  align-items:center;
  gap:8px;
}
.tool-name .icon{
  width:6px;height:6px;
  border-radius:50%;
  flex-shrink:0;
}
.tool-desc{
  font-size:13px;
  color:var(--text-secondary);
  line-height:1.45;
}

/* ── Connection ── */
.connect-grid{
  display:grid;
  grid-template-columns:1fr 1fr;
  gap:24px;
}
@media(max-width:640px){
  .connect-grid{grid-template-columns:1fr}
}
.connect-card{
  background:var(--surface);
  border-radius:var(--radius);
  padding:24px;
  box-shadow:var(--shadow);
}
.connect-card h3{
  font-size:15px;
  font-weight:600;
  margin-bottom:16px;
  display:flex;
  align-items:center;
  gap:8px;
}
.connect-card h3 .num{
  width:22px;height:22px;
  background:var(--blue);
  color:#fff;
  border-radius:50%;
  display:inline-flex;
  align-items:center;justify-content:center;
  font-size:12px;
  font-weight:700;
  flex-shrink:0;
}
pre{
  background:var(--code-bg);
  border-radius:var(--radius-sm);
  padding:16px;
  font-size:13px;
  line-height:1.6;
  overflow-x:auto;
  font-family:"SF Mono",SFMono-Regular,Menlo,Monaco,Consolas,monospace;
  color:var(--text);
}
.env-list{list-style:none;padding:0}
.env-list li{
  padding:10px 0;
  border-bottom:1px solid var(--divider);
  display:flex;
  gap:12px;
  align-items:baseline;
  font-size:14px;
}
.env-list li:last-child{border-bottom:none}
.env-var{
  font-family:"SF Mono",SFMono-Regular,Menlo,Monaco,Consolas,monospace;
  font-weight:600;
  font-size:13px;
  color:var(--blue);
  flex-shrink:0;
}
.env-desc{
  color:var(--text-secondary);
  font-size:13px;
}

/* ── Footer ── */
footer{
  padding:32px 0;
  border-top:1px solid var(--divider);
  text-align:center;
}
footer p{
  font-size:13px;
  color:var(--text-secondary);
}
footer a{
  color:var(--blue);
  text-decoration:none;
}
footer a:hover{text-decoration:underline}

/* ── Animations ── */
@keyframes fadeUp{
  from{opacity:0;transform:translateY(16px)}
  to{opacity:1;transform:translateY(0)}
}
.fade-in{
  animation:fadeUp 0.5s ease both;
}
.fade-in.d1{animation-delay:0.05s}
.fade-in.d2{animation-delay:0.1s}
.fade-in.d3{animation-delay:0.15s}
.fade-in.d4{animation-delay:0.2s}

/* ── Responsive ── */
@media(max-width:600px){
  .hero{padding:48px 0 40px}
  .hero h2{font-size:28px}
  .hero p{font-size:16px}
  .section-title{font-size:22px}
  .tools-grid{grid-template-columns:1fr}
}
</style>
</head>
<body>

<!-- Header -->
<header>
  <div class="container header-inner">
    <div class="logo"><div class="checkmark"></div></div>
    <div class="header-text">
      <h1>Things Cloud MCP</h1>
      <p>Model Context Protocol server for Things 3</p>
    </div>
  </div>
</header>

<!-- Hero -->
<section class="hero">
  <div class="container">
    <div class="hero-icon fade-in"><div class="checkmark-lg"></div></div>
    <h2 class="fade-in d1">Connect your AI to<br><span class="gradient">Things 3</span></h2>
    <p class="fade-in d2">A Model Context Protocol server that gives your AI assistant full access to Things 3 task management through Things Cloud.</p>
    <div class="hero-meta fade-in d3">
      <span class="badge"><span class="dot"></span> Streamable HTTP</span>
      <span class="badge">localhost:8080/mcp</span>
      <span class="badge">13 Tools</span>
    </div>
  </div>
</section>

<!-- Tools -->
<section class="alt">
  <div class="container">
    <div class="section-header">
      <div class="section-label">Capabilities</div>
      <div class="section-title">Built-in Tools</div>
      <div class="section-desc">Everything you need to manage your tasks, projects, and areas from any MCP-compatible AI assistant.</div>
    </div>

    <!-- Read Tools -->
    <div class="tools-category fade-in">
      <div class="category-label"><span class="category-dot read"></span> Read</div>
      <div class="tools-grid">
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#007AFF"></span>list_tasks</div>
          <div class="tool-desc">List tasks filtered by status, project, area, or tag</div>
        </div>
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#007AFF"></span>show_task</div>
          <div class="tool-desc">Get full details of a specific task including checklist items and notes</div>
        </div>
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#007AFF"></span>list_projects</div>
          <div class="tool-desc">List all active projects with progress information</div>
        </div>
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#007AFF"></span>list_areas</div>
          <div class="tool-desc">List all areas of responsibility</div>
        </div>
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#007AFF"></span>list_tags</div>
          <div class="tool-desc">List all available tags for organizing tasks</div>
        </div>
      </div>
    </div>

    <!-- Create Tools -->
    <div class="tools-category fade-in d1">
      <div class="category-label"><span class="category-dot create"></span> Create</div>
      <div class="tools-grid">
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#34C759"></span>create_task</div>
          <div class="tool-desc">Create a new task with title, notes, tags, deadline, and checklist</div>
        </div>
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#34C759"></span>create_area</div>
          <div class="tool-desc">Create a new area of responsibility</div>
        </div>
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#34C759"></span>create_tag</div>
          <div class="tool-desc">Create a new tag for task organization</div>
        </div>
      </div>
    </div>

    <!-- Modify Tools -->
    <div class="tools-category fade-in d2">
      <div class="category-label"><span class="category-dot modify"></span> Modify</div>
      <div class="tools-grid">
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#FF9500"></span>edit_task</div>
          <div class="tool-desc">Update a task&#39;s title, notes, tags, deadline, or checklist items</div>
        </div>
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#FF9500"></span>complete_task</div>
          <div class="tool-desc">Mark a task as complete</div>
        </div>
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#FF9500"></span>trash_task</div>
          <div class="tool-desc">Move a task to the trash</div>
        </div>
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#FF9500"></span>move_to_today</div>
          <div class="tool-desc">Move a task to the Today list for immediate focus</div>
        </div>
      </div>
    </div>

    <!-- Advanced -->
    <div class="tools-category fade-in d3">
      <div class="category-label"><span class="category-dot advanced"></span> Advanced</div>
      <div class="tools-grid">
        <div class="tool-card">
          <div class="tool-name"><span class="icon" style="background:#AF52DE"></span>batch_operations</div>
          <div class="tool-desc">Perform multiple operations in a single request for efficiency</div>
        </div>
      </div>
    </div>
  </div>
</section>

<!-- Connection -->
<section>
  <div class="container">
    <div class="section-header">
      <div class="section-label">Get Started</div>
      <div class="section-title">Connect in Seconds</div>
      <div class="section-desc">Add the server to your MCP client configuration and set your credentials.</div>
    </div>

    <div class="connect-grid">
      <div class="connect-card fade-in">
        <h3><span class="num">1</span> MCP Configuration</h3>
        <pre>{
  "mcpServers": {
    "things-cloud": {
      "url": "http://localhost:8080/mcp"
    }
  }
}</pre>
      </div>
      <div class="connect-card fade-in d1">
        <h3><span class="num">2</span> Environment Variables</h3>
        <ul class="env-list">
          <li>
            <span class="env-var">THINGS_USERNAME</span>
            <span class="env-desc">Your Things Cloud account email</span>
          </li>
          <li>
            <span class="env-var">THINGS_PASSWORD</span>
            <span class="env-desc">Your Things Cloud account password</span>
          </li>
        </ul>
      </div>
    </div>
  </div>
</section>

<!-- Footer -->
<footer>
  <div class="container">
    <p>Powered by <a href="https://github.com/arthursoares/things-cloud-sdk" target="_blank" rel="noopener">Things Cloud SDK</a></p>
  </div>
</footer>

</body>
</html>`

func handleLandingPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(LandingPageHTML))
}
