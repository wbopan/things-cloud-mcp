package main

import (
	"encoding/base64"
	"net/http"
	"strings"
)

// sharedCSS contains the CSS shared between the landing page and docs page.
const sharedCSS = `*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}

:root{
  --blue:#1A7CF9;
  --blue-hover:#0A6CE9;
  --bg:#FFFFFF;
  --bg-alt:#F7F7F7;
  --text:#1C1C1E;
  --text-secondary:#8E8E93;
  --green:#34C759;
  --surface:#FFFFFF;
  --shadow:0 0.5px 1px rgba(0,0,0,0.05);
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
    --shadow:0 0.5px 1px rgba(0,0,0,0.15);
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

/* -- Header -- */
header{
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
  display:flex;align-items:center;justify-content:center;
  flex-shrink:0;
}
.logo svg{width:40px;height:40px}
.header-text h1{
  font-size:20px;
  font-weight:700;
  letter-spacing:-0.3px;
  color:var(--text);
}

/* -- Hero -- */
.hero{
  padding:80px 0 64px;
  text-align:center;
}
.hero-icon{
  width:80px;height:80px;
  display:inline-flex;align-items:center;justify-content:center;
  margin-bottom:28px;
}
.hero-icon svg{width:80px;height:80px}
.hero h2{
  font-size:40px;
  font-weight:700;
  letter-spacing:-0.8px;
  margin-bottom:16px;
  line-height:1.15;
}
.hero h2 .brand{
  color:var(--blue);
}
.hero p{
  font-size:18px;
  color:var(--text-secondary);
  max-width:540px;
  margin:0 auto 24px;
  line-height:1.55;
}
.hero-features{
  display:flex;
  justify-content:center;
  align-items:center;
  gap:10px;
  margin-bottom:0;
}
.hero-feature{
  font-size:13px;
  font-weight:500;
  color:var(--text-secondary);
  letter-spacing:0.2px;
}
.hero-feature-sep{
  font-size:13px;
  color:var(--divider);
  font-weight:300;
}
/* -- Header right-side items -- */
.header-right{
  display:flex;
  align-items:center;
  gap:12px;
  margin-left:auto;
}
.header-status{
  display:inline-flex;
  align-items:center;
  gap:6px;
  font-size:13px;
  font-weight:400;
  color:var(--text-secondary);
}
.header-status .dot{
  width:6px;height:6px;
  border-radius:50%;
  background:var(--green);
  flex-shrink:0;
}
.header-divider{
  color:var(--divider);
  font-size:13px;
  font-weight:300;
  user-select:none;
}
.header-doc-link{
  font-size:13px;
  font-weight:400;
  color:var(--text-secondary);
  text-decoration:none;
}
.header-doc-link:hover{color:var(--blue)}

/* -- Section shared -- */
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

/* -- Connection -- */
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
  border:1px solid var(--divider);
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

/* -- Client Selector Dropdown -- */
.client-selector-wrapper{
  position:relative;
  display:inline-block;
}
.client-selector-trigger{
  color:var(--blue);
  cursor:pointer;
  font-weight:700;
  font-size:inherit;
  letter-spacing:inherit;
  user-select:none;
  -webkit-user-select:none;
  display:inline-flex;
  align-items:center;
  vertical-align:-6px;
  gap:6px;
}
.client-selector-trigger svg{
  width:32px;height:32px;
}
.client-selector-trigger:hover{
  color:var(--blue-hover);
}
.client-dropdown{
  display:none;
  position:absolute;
  top:calc(100% + 6px);
  left:50%;
  transform:translateX(-50%);
  min-width:200px;
  background:var(--surface);
  border:1px solid var(--divider);
  border-radius:var(--radius-sm);
  box-shadow:0 4px 16px rgba(0,0,0,0.08);
  z-index:200;
  overflow:hidden;
}
@media(prefers-color-scheme:dark){
  .client-dropdown{
    box-shadow:0 4px 16px rgba(0,0,0,0.25);
  }
}
.client-dropdown.open{
  display:block;
}
.client-dropdown-item{
  padding:10px 16px;
  font-size:15px;
  font-weight:500;
  color:var(--text);
  cursor:pointer;
  transition:background 0.15s;
  display:inline-flex;
  align-items:center;
  gap:6px;
  width:100%;
}
.client-dropdown-item svg{
  flex-shrink:0;
}
#clientLabel,#docsClientLabel{
  display:inline-flex;
  align-items:center;
  gap:6px;
}
#clientLabel svg,#docsClientLabel svg{
  flex-shrink:0;
  position:relative;
  top:1px;
}
.client-dropdown-item:hover{
  background:var(--bg-alt);
}
.client-dropdown-item.active{
  color:var(--blue);
}
.client-instructions-container{
  background:var(--surface);
  border-radius:var(--radius);
  padding:28px 24px;
  border:1px solid var(--divider);
}
.client-instructions{
  display:none;
}
.client-instructions.active{
  display:block;
}
.client-instructions .step{
  display:flex;
  gap:12px;
  margin-bottom:18px;
  align-items:flex-start;
  font-size:15px;
  line-height:1.6;
}
.client-instructions .step:last-child{
  margin-bottom:0;
}
.client-instructions .step .num{
  width:22px;height:22px;
  min-width:22px;
  background:var(--blue);
  color:#fff;
  border-radius:50%;
  display:inline-flex;
  align-items:center;justify-content:center;
  font-size:12px;
  font-weight:700;
  flex-shrink:0;
  margin-top:2px;
}
.client-instructions .step-text{
  flex:1;
}
.client-instructions .step-text strong{
  font-weight:600;
}
.client-instructions pre{
  margin-top:12px;
  margin-bottom:12px;
}
.client-instructions .note{
  font-size:13px;
  color:var(--text-secondary);
  margin-top:12px;
  line-height:1.55;
}

/* -- Footer -- */
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

/* -- Animations -- */
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

/* -- Responsive -- */
@media(max-width:600px){
  .hero{padding:48px 0 40px}
  .hero h2{font-size:28px}
  .hero p{font-size:16px}
  .section-title{font-size:22px}
}
`

// cloudCheckSVGSmall is the header logo SVG (40x40).
const cloudCheckSVGSmall = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="40" height="40" fill="none">
  <path d="M50 46a11 11 0 0 0 0-22 11 11 0 0 0-1-.04 15 15 0 0 0-29-2A13 13 0 0 0 14 46h36z" fill="#1A7CF9"/>
  <polyline points="24,34 30,40 42,28" stroke="#fff" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
</svg>`

// cloudCheckSVGLarge is the hero icon SVG (80x80).
const cloudCheckSVGLarge = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="80" height="80" fill="none">
  <path d="M50 46a11 11 0 0 0 0-22 11 11 0 0 0-1-.04 15 15 0 0 0-29-2A13 13 0 0 0 14 46h36z" fill="#1A7CF9"/>
  <polyline points="24,34 30,40 42,28" stroke="#fff" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
</svg>`

// LandingPageHTML contains the full HTML landing page styled after Things 3's design language.
var LandingPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="icon" type="image/png" sizes="32x32" href="/favicon.ico">
<link rel="icon" type="image/svg+xml" href="/favicon.svg">
<title>Things Cloud MCP</title>
<style>
` + sharedCSS + `
</style>
</head>
<body>

<!-- Header -->
<header>
  <div class="container header-inner">
    <div class="logo">` + cloudCheckSVGSmall + `</div>
    <div class="header-text">
      <h1>Things Cloud MCP</h1>
    </div>
    <div class="header-right">
      <span class="header-status"><span class="dot"></span> Online</span>
      <span class="header-divider">|</span>
      <a class="header-doc-link" href="/how-it-works">How it Works</a>
      <span class="header-divider">|</span>
      <a class="header-doc-link" href="/docs">Documentation</a>
    </div>
  </div>
</header>

<!-- Hero -->
<section class="hero">
  <div class="container">
    <div class="hero-icon fade-in">` + cloudCheckSVGLarge + `</div>
    <h2 class="fade-in d1">Connect your AI to <span class="brand">Things 3</span></h2>
    <p class="fade-in d2">A Model Context Protocol server that gives your AI assistant full access to Things 3 task management through Things Cloud.</p>
    <div class="hero-features fade-in d3">
      <span class="hero-feature">Streamable HTTP</span>
      <span class="hero-feature-sep">/</span>
      <span class="hero-feature">OAuth 2.0</span>
      <span class="hero-feature-sep">/</span>
      <span class="hero-feature">19 Tools</span>
    </div>
  </div>
</section>

<!-- Connection -->
<section>
  <div class="container">
    <div class="section-header">
      <div class="section-label">Get Started</div>
      <div class="section-title">Setup MCP with <span class="client-selector-wrapper"><span class="client-selector-trigger" id="clientTrigger"><span id="clientLabel">Claude.ai</span> &#9662;</span><div class="client-dropdown" id="clientDropdown"><div class="client-dropdown-item active" data-client="claude-ai"><svg width="16" height="16" viewBox="0 0 24 24" fill="#d97757" xmlns="http://www.w3.org/2000/svg"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg> Claude.ai</div><div class="client-dropdown-item" data-client="claude-code"><svg width="16" height="16" viewBox="0 0 24 24" fill="#d97757" xmlns="http://www.w3.org/2000/svg"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg> Claude Code</div><div class="client-dropdown-item" data-client="chatgpt"><svg width="16" height="16" viewBox="0 0 16 16" fill="#74AA9C" xmlns="http://www.w3.org/2000/svg"><path d="M14.949 6.547a3.94 3.94 0 0 0-.348-3.273 4.11 4.11 0 0 0-4.4-1.934A4.1 4.1 0 0 0 8.423.2 4.15 4.15 0 0 0 6.305.086a4.1 4.1 0 0 0-1.891.948 4.04 4.04 0 0 0-1.158 1.753 4.1 4.1 0 0 0-1.563.679A4 4 0 0 0 .554 4.72a3.99 3.99 0 0 0 .502 4.731 3.94 3.94 0 0 0 .346 3.274 4.11 4.11 0 0 0 4.402 1.933c.382.425.852.764 1.377.995.526.231 1.095.35 1.67.346 1.78.002 3.358-1.132 3.901-2.804a4.1 4.1 0 0 0 1.563-.68 4 4 0 0 0 1.14-1.253 3.99 3.99 0 0 0-.506-4.716m-6.097 8.406a3.05 3.05 0 0 1-1.945-.694l.096-.054 3.23-1.838a.53.53 0 0 0 .265-.455v-4.49l1.366.778q.02.011.025.035v3.722c-.003 1.653-1.361 2.992-3.037 2.996m-6.53-2.75a2.95 2.95 0 0 1-.36-2.01l.095.057L5.29 12.09a.53.53 0 0 0 .527 0l3.949-2.246v1.555a.05.05 0 0 1-.022.041L6.473 13.3c-1.454.826-3.311.335-4.15-1.098m-.85-6.94A3.02 3.02 0 0 1 3.07 3.949v3.785a.51.51 0 0 0 .262.451l3.93 2.237-1.366.779a.05.05 0 0 1-.048 0L2.585 9.342a2.98 2.98 0 0 1-1.113-4.094zm11.216 2.571L8.747 5.576l1.362-.776a.05.05 0 0 1 .048 0l3.265 1.86a3 3 0 0 1 1.173 1.207 2.96 2.96 0 0 1-.27 3.2 3.05 3.05 0 0 1-1.36.997V8.279a.52.52 0 0 0-.276-.445m1.36-2.015-.097-.057-3.226-1.855a.53.53 0 0 0-.53 0L6.249 6.153V4.598a.04.04 0 0 1 .019-.04L9.533 2.7a3.07 3.07 0 0 1 3.257.139c.474.325.843.778 1.066 1.303.223.526.289 1.103.191 1.664zM5.503 8.575 4.139 7.8a.05.05 0 0 1-.026-.037V4.049c0-.57.166-1.127.476-1.607s.752-.864 1.275-1.105a3.08 3.08 0 0 1 3.234.41l-.096.054-3.23 1.838a.53.53 0 0 0-.265.455zm.742-1.577 1.758-1 1.762 1v2l-1.755 1-1.762-1z"/></svg> ChatGPT</div><div class="client-dropdown-item" data-client="cursor"><svg width="16" height="16" viewBox="0 0 24 24" fill="#F54E00" xmlns="http://www.w3.org/2000/svg"><path d="M11.503.131 1.891 5.678a.84.84 0 0 0-.42.726v11.188c0 .3.162.575.42.724l9.609 5.55a1 1 0 0 0 .998 0l9.61-5.55a.84.84 0 0 0 .42-.724V6.404a.84.84 0 0 0-.42-.726L12.497.131a1.01 1.01 0 0 0-.996 0M2.657 6.338h18.55c.263 0 .43.287.297.515L12.23 22.918c-.062.107-.229.064-.229-.06V12.335a.59.59 0 0 0-.295-.51l-9.11-5.257c-.109-.063-.064-.23.061-.23"/></svg> Cursor</div><div class="client-dropdown-item" data-client="windsurf"><svg width="16" height="16" viewBox="0 0 24 24" fill="#06B6D4" xmlns="http://www.w3.org/2000/svg"><path d="M23.55 5.067c-1.2038-.002-2.1806.973-2.1806 2.1765v4.8676c0 .972-.8035 1.7594-1.7597 1.7594-.568 0-1.1352-.286-1.4718-.7659l-4.9713-7.1003c-.4125-.5896-1.0837-.941-1.8103-.941-1.1334 0-2.1533.9635-2.1533 2.153v4.8957c0 .972-.7969 1.7594-1.7596 1.7594-.57 0-1.1363-.286-1.4728-.7658L.4076 5.1598C.2822 4.9798 0 5.0688 0 5.2882v4.2452c0 .2147.0656.4228.1884.599l5.4748 7.8183c.3234.462.8006.8052 1.3509.9298 1.3771.313 2.6446-.747 2.6446-2.0977v-4.893c0-.972.7875-1.7593 1.7596-1.7593h.003a1.798 1.798 0 0 1 1.4718.7658l4.9723 7.0994c.4135.5905 1.05.941 1.8093.941 1.1587 0 2.1515-.9645 2.1515-2.153v-4.8948c0-.972.7875-1.7594 1.7596-1.7594h.194a.22.22 0 0 0 .2204-.2202v-4.622a.22.22 0 0 0-.2203-.2203Z"/></svg> Windsurf</div></div></span></div>
    </div>

    <div class="client-instructions-container fade-in">

      <div class="client-instructions active" data-client="claude-ai">
        <div class="step"><span class="num">1</span><div class="step-text">Go to <strong>Settings &rarr; Connectors &rarr; Add custom connector</strong></div></div>
        <div class="step"><span class="num">2</span><div class="step-text">Enter name: <strong>Things Cloud</strong></div></div>
        <div class="step"><span class="num">3</span><div class="step-text">Enter URL: <strong><span class="mcp-url"></span></strong></div></div>
        <div class="step"><span class="num">4</span><div class="step-text">Click <strong>Add</strong>, then enable in chat via the &ldquo;+&rdquo; button</div></div>
      </div>

      <div class="client-instructions" data-client="claude-code">
        <div class="step"><span class="num">1</span><div class="step-text">Run the following command:</div></div>
        <pre><code>claude mcp add --transport http \
  --header "Authorization: Basic BASE64_ENCODE(email:password)" \
  things-cloud <span class="mcp-url"></span></code></pre>
        <div class="note">Replace <strong>BASE64_ENCODE(email:password)</strong> with your base64-encoded Things Cloud credentials (email:password). Generate with: <code>echo -n 'email:password' | base64</code></div>
        <div class="step"><span class="num">2</span><div class="step-text">Verify with the <strong>/mcp</strong> command inside Claude Code.</div></div>
      </div>

      <div class="client-instructions" data-client="chatgpt">
        <div class="step"><span class="num">1</span><div class="step-text">Go to <strong>Settings &rarr; Apps &amp; Connectors &rarr; Advanced</strong>, enable <strong>Developer Mode</strong></div></div>
        <div class="step"><span class="num">2</span><div class="step-text">Click <strong>Add Connector</strong></div></div>
        <div class="step"><span class="num">3</span><div class="step-text">Enter name: <strong>Things Cloud</strong>, URL: <strong><span class="mcp-url"></span></strong></div></div>
        <div class="step"><span class="num">4</span><div class="step-text">In a new chat, click &ldquo;+&rdquo; to select the connector</div></div>
        <div class="note">Note: ChatGPT requires a publicly accessible URL (use ngrok for local dev).</div>
      </div>

      <div class="client-instructions" data-client="cursor">
        <div class="step"><span class="num">1</span><div class="step-text">Add to <strong>~/.cursor/mcp.json</strong>:</div></div>
        <pre><code>{
  "mcpServers": {
    "things-cloud": {
      "url": "<span class="mcp-url"></span>",
      "headers": {
        "Authorization": "Basic BASE64_ENCODE(email:password)"
      }
    }
  }
}</code></pre>
        <div class="note">Replace <strong>BASE64_ENCODE(email:password)</strong> with your base64-encoded Things Cloud credentials (email:password). Generate with: <code>echo -n 'email:password' | base64</code></div>
      </div>

      <div class="client-instructions" data-client="windsurf">
        <div class="step"><span class="num">1</span><div class="step-text">Add to <strong>~/.codeium/windsurf/mcp_config.json</strong>:</div></div>
        <pre><code>{
  "mcpServers": {
    "things-cloud": {
      "serverUrl": "<span class="mcp-url"></span>",
      "headers": {
        "Authorization": "Basic BASE64_ENCODE(email:password)"
      }
    }
  }
}</code></pre>
        <div class="note">Replace <strong>BASE64_ENCODE(email:password)</strong> with your base64-encoded Things Cloud credentials (email:password). Generate with: <code>echo -n 'email:password' | base64</code></div>
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

<script>
(function(){
  var mcpUrl = window.location.origin + "/mcp";
  document.querySelectorAll(".mcp-url").forEach(function(el){ el.textContent = mcpUrl; });
})();
(function(){
  var iconMap = {
    "claude-ai": '<svg width="16" height="16" viewBox="0 0 24 24" fill="#d97757" xmlns="http://www.w3.org/2000/svg"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg>',
    "claude-code": '<svg width="16" height="16" viewBox="0 0 24 24" fill="#d97757" xmlns="http://www.w3.org/2000/svg"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg>',
    "chatgpt": '<svg width="16" height="16" viewBox="0 0 16 16" fill="#74AA9C" xmlns="http://www.w3.org/2000/svg"><path d="M14.949 6.547a3.94 3.94 0 0 0-.348-3.273 4.11 4.11 0 0 0-4.4-1.934A4.1 4.1 0 0 0 8.423.2 4.15 4.15 0 0 0 6.305.086a4.1 4.1 0 0 0-1.891.948 4.04 4.04 0 0 0-1.158 1.753 4.1 4.1 0 0 0-1.563.679A4 4 0 0 0 .554 4.72a3.99 3.99 0 0 0 .502 4.731 3.94 3.94 0 0 0 .346 3.274 4.11 4.11 0 0 0 4.402 1.933c.382.425.852.764 1.377.995.526.231 1.095.35 1.67.346 1.78.002 3.358-1.132 3.901-2.804a4.1 4.1 0 0 0 1.563-.68 4 4 0 0 0 1.14-1.253 3.99 3.99 0 0 0-.506-4.716m-6.097 8.406a3.05 3.05 0 0 1-1.945-.694l.096-.054 3.23-1.838a.53.53 0 0 0 .265-.455v-4.49l1.366.778q.02.011.025.035v3.722c-.003 1.653-1.361 2.992-3.037 2.996m-6.53-2.75a2.95 2.95 0 0 1-.36-2.01l.095.057L5.29 12.09a.53.53 0 0 0 .527 0l3.949-2.246v1.555a.05.05 0 0 1-.022.041L6.473 13.3c-1.454.826-3.311.335-4.15-1.098m-.85-6.94A3.02 3.02 0 0 1 3.07 3.949v3.785a.51.51 0 0 0 .262.451l3.93 2.237-1.366.779a.05.05 0 0 1-.048 0L2.585 9.342a2.98 2.98 0 0 1-1.113-4.094zm11.216 2.571L8.747 5.576l1.362-.776a.05.05 0 0 1 .048 0l3.265 1.86a3 3 0 0 1 1.173 1.207 2.96 2.96 0 0 1-.27 3.2 3.05 3.05 0 0 1-1.36.997V8.279a.52.52 0 0 0-.276-.445m1.36-2.015-.097-.057-3.226-1.855a.53.53 0 0 0-.53 0L6.249 6.153V4.598a.04.04 0 0 1 .019-.04L9.533 2.7a3.07 3.07 0 0 1 3.257.139c.474.325.843.778 1.066 1.303.223.526.289 1.103.191 1.664zM5.503 8.575 4.139 7.8a.05.05 0 0 1-.026-.037V4.049c0-.57.166-1.127.476-1.607s.752-.864 1.275-1.105a3.08 3.08 0 0 1 3.234.41l-.096.054-3.23 1.838a.53.53 0 0 0-.265.455zm.742-1.577 1.758-1 1.762 1v2l-1.755 1-1.762-1z"/></svg>',
    "cursor": '<svg width="16" height="16" viewBox="0 0 24 24" fill="#F54E00" xmlns="http://www.w3.org/2000/svg"><path d="M11.503.131 1.891 5.678a.84.84 0 0 0-.42.726v11.188c0 .3.162.575.42.724l9.609 5.55a1 1 0 0 0 .998 0l9.61-5.55a.84.84 0 0 0 .42-.724V6.404a.84.84 0 0 0-.42-.726L12.497.131a1.01 1.01 0 0 0-.996 0M2.657 6.338h18.55c.263 0 .43.287.297.515L12.23 22.918c-.062.107-.229.064-.229-.06V12.335a.59.59 0 0 0-.295-.51l-9.11-5.257c-.109-.063-.064-.23.061-.23"/></svg>',
    "windsurf": '<svg width="16" height="16" viewBox="0 0 24 24" fill="#06B6D4" xmlns="http://www.w3.org/2000/svg"><path d="M23.55 5.067c-1.2038-.002-2.1806.973-2.1806 2.1765v4.8676c0 .972-.8035 1.7594-1.7597 1.7594-.568 0-1.1352-.286-1.4718-.7659l-4.9713-7.1003c-.4125-.5896-1.0837-.941-1.8103-.941-1.1334 0-2.1533.9635-2.1533 2.153v4.8957c0 .972-.7969 1.7594-1.7596 1.7594-.57 0-1.1363-.286-1.4728-.7658L.4076 5.1598C.2822 4.9798 0 5.0688 0 5.2882v4.2452c0 .2147.0656.4228.1884.599l5.4748 7.8183c.3234.462.8006.8052 1.3509.9298 1.3771.313 2.6446-.747 2.6446-2.0977v-4.893c0-.972.7875-1.7593 1.7596-1.7593h.003a1.798 1.798 0 0 1 1.4718.7658l4.9723 7.0994c.4135.5905 1.05.941 1.8093.941 1.1587 0 2.1515-.9645 2.1515-2.153v-4.8948c0-.972.7875-1.7594 1.7596-1.7594h.194a.22.22 0 0 0 .2204-.2202v-4.622a.22.22 0 0 0-.2203-.2203Z"/></svg>'
  };
  var nameMap = {
    "claude-ai": "Claude.ai",
    "claude-code": "Claude Code",
    "chatgpt": "ChatGPT",
    "cursor": "Cursor",
    "windsurf": "Windsurf"
  };

  var trigger = document.getElementById("clientTrigger");
  var dropdown = document.getElementById("clientDropdown");
  var label = document.getElementById("clientLabel");
  var items = dropdown.querySelectorAll(".client-dropdown-item");
  var allInstructions = document.querySelectorAll(".client-instructions");

  label.innerHTML = iconMap["claude-ai"] + " " + nameMap["claude-ai"];

  trigger.addEventListener("click", function(e){
    e.stopPropagation();
    dropdown.classList.toggle("open");
  });

  items.forEach(function(item){
    item.addEventListener("click", function(e){
      e.stopPropagation();
      var clientId = this.getAttribute("data-client");
      label.innerHTML = iconMap[clientId] + " " + nameMap[clientId];
      items.forEach(function(el){ el.classList.remove("active"); });
      this.classList.add("active");
      allInstructions.forEach(function(el){
        if(el.getAttribute("data-client") === clientId){
          el.classList.add("active");
        } else {
          el.classList.remove("active");
        }
      });
      dropdown.classList.remove("open");
    });
  });

  document.addEventListener("click", function(){
    dropdown.classList.remove("open");
  });
})();
</script>

</body>
</html>`

// DocsPageHTML contains the full HTML documentation page.
var DocsPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="icon" type="image/png" sizes="32x32" href="/favicon.ico">
<link rel="icon" type="image/svg+xml" href="/favicon.svg">
<title>Documentation - Things Cloud MCP</title>
<style>
` + sharedCSS + `

/* -- Docs-specific styles -- */
.docs-nav{
  padding:24px 0 0;
}
.docs-nav a{
  color:var(--blue);
  text-decoration:none;
  font-size:14px;
  font-weight:500;
}
.docs-nav a:hover{text-decoration:underline}

.docs-hero{
  padding:48px 0 40px;
}
.docs-hero h2{
  font-size:32px;
  font-weight:700;
  letter-spacing:-0.5px;
  margin-bottom:8px;
}
.docs-hero p{
  font-size:16px;
  color:var(--text-secondary);
  max-width:540px;
  line-height:1.55;
}

.docs-section{
  padding:0 0 48px;
}
.docs-section:last-of-type{
  padding-bottom:64px;
}

.category-header{
  display:flex;
  align-items:center;
  gap:10px;
  margin-bottom:20px;
  padding-bottom:12px;
  border-bottom:1px solid var(--divider);
}
.category-dot{
  width:8px;height:8px;
  border-radius:50%;
  flex-shrink:0;
}
.category-dot.read{background:#007AFF}
.category-dot.create{background:#34C759}
.category-dot.modify{background:#FF9500}
.category-header h3{
  font-size:18px;
  font-weight:600;
  letter-spacing:-0.2px;
}
.category-header .count{
  font-size:13px;
  color:var(--text-secondary);
  font-weight:400;
}

.tool-entry{
  margin-bottom:28px;
}
.tool-entry:last-child{
  margin-bottom:0;
}
.tool-entry-name{
  font-size:15px;
  font-weight:600;
  font-family:"SF Mono",SFMono-Regular,Menlo,Monaco,Consolas,monospace;
  color:var(--text);
  margin-bottom:4px;
}
.tool-entry-desc{
  font-size:14px;
  color:var(--text-secondary);
  margin-bottom:10px;
}
.params-table{
  width:100%;
  border-collapse:collapse;
  font-size:13px;
}
.params-table th{
  text-align:left;
  padding:8px 12px;
  background:var(--bg-alt);
  font-weight:600;
  color:var(--text-secondary);
  font-size:12px;
  text-transform:uppercase;
  letter-spacing:0.5px;
  border-bottom:1px solid var(--divider);
}
.params-table td{
  padding:8px 12px;
  border-bottom:1px solid var(--divider);
  vertical-align:top;
}
.params-table tr:last-child td{
  border-bottom:none;
}
.param-name{
  font-family:"SF Mono",SFMono-Regular,Menlo,Monaco,Consolas,monospace;
  font-weight:600;
  color:var(--blue);
  white-space:nowrap;
}
.param-required{
  color:#FF3B30;
  font-size:11px;
  font-weight:600;
  margin-left:4px;
}
.param-type{
  font-family:"SF Mono",SFMono-Regular,Menlo,Monaco,Consolas,monospace;
  font-size:12px;
  color:var(--text-secondary);
}
.no-params{
  font-size:13px;
  color:var(--text-secondary);
  font-style:italic;
}

.output-section{
  padding:0 0 48px;
}
.output-section h3{
  font-size:18px;
  font-weight:600;
  letter-spacing:-0.2px;
  margin-bottom:8px;
}
.output-section p{
  font-size:14px;
  color:var(--text-secondary);
  margin-bottom:16px;
}
</style>
</head>
<body>

<!-- Header -->
<header>
  <div class="container header-inner">
    <div class="logo">` + cloudCheckSVGSmall + `</div>
    <div class="header-text">
      <h1>Things Cloud MCP</h1>
    </div>
    <div class="header-right">
      <span class="header-status"><span class="dot"></span> Online</span>
      <span class="header-divider">|</span>
      <a class="header-doc-link" href="/how-it-works">How it Works</a>
      <span class="header-divider">|</span>
      <a class="header-doc-link" href="/docs">Documentation</a>
    </div>
  </div>
</header>

<div class="container">

<!-- Back nav -->
<div class="docs-nav">
  <a href="/">&larr; Back to home</a>
</div>

<!-- Docs Hero -->
<div class="docs-hero">
  <h2>Documentation</h2>
  <p>Complete reference for all 19 tools available through the Things Cloud MCP server.</p>
</div>

<!-- Read Tools -->
<div class="docs-section">
  <div class="category-header">
    <span class="category-dot read"></span>
    <h3>Read</h3>
    <span class="count">7 tools</span>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">list_tasks</div>
    <div class="tool-entry-desc">List tasks with optional filters</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">schedule</span></td><td class="param-type">enum</td><td>inbox, today, anytime, someday, upcoming</td></tr>
      <tr><td><span class="param-name">scheduled_before</span></td><td class="param-type">string</td><td>YYYY-MM-DD</td></tr>
      <tr><td><span class="param-name">scheduled_after</span></td><td class="param-type">string</td><td>YYYY-MM-DD</td></tr>
      <tr><td><span class="param-name">deadline_before</span></td><td class="param-type">string</td><td>YYYY-MM-DD</td></tr>
      <tr><td><span class="param-name">deadline_after</span></td><td class="param-type">string</td><td>YYYY-MM-DD</td></tr>
      <tr><td><span class="param-name">tag</span></td><td class="param-type">string</td><td>Filter by tag</td></tr>
      <tr><td><span class="param-name">area</span></td><td class="param-type">string</td><td>Filter by area</td></tr>
      <tr><td><span class="param-name">project</span></td><td class="param-type">string</td><td>Filter by project</td></tr>
      <tr><td><span class="param-name">in_trash</span></td><td class="param-type">bool</td><td>Include trashed items (default false)</td></tr>
      <tr><td><span class="param-name">is_completed</span></td><td class="param-type">bool</td><td>Include completed items (default false)</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">show_task</div>
    <div class="tool-entry-desc">Show task details including checklist. Accepts UUID prefix.</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Task UUID or prefix</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">show_project</div>
    <div class="tool-entry-desc">Show project with headings and grouped tasks</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Project UUID or prefix</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">list_projects</div>
    <div class="tool-entry-desc">List all active projects</div>
    <div class="no-params">No parameters</div>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">list_headings</div>
    <div class="tool-entry-desc">List headings in a project</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">project_uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Project UUID</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">list_areas</div>
    <div class="tool-entry-desc">List all areas</div>
    <div class="no-params">No parameters</div>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">list_tags</div>
    <div class="tool-entry-desc">List all tags</div>
    <div class="no-params">No parameters</div>
  </div>
</div>

<!-- Create Tools -->
<div class="docs-section">
  <div class="category-header">
    <span class="category-dot create"></span>
    <h3>Create</h3>
    <span class="count">5 tools</span>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">create_task</div>
    <div class="tool-entry-desc">Create a task</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">title</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Task title</td></tr>
      <tr><td><span class="param-name">note</span></td><td class="param-type">string</td><td>Task notes</td></tr>
      <tr><td><span class="param-name">schedule</span></td><td class="param-type">enum</td><td>today, anytime, someday, inbox</td></tr>
      <tr><td><span class="param-name">deadline</span></td><td class="param-type">string</td><td>YYYY-MM-DD</td></tr>
      <tr><td><span class="param-name">scheduled</span></td><td class="param-type">string</td><td>YYYY-MM-DD</td></tr>
      <tr><td><span class="param-name">project_uuid</span></td><td class="param-type">string</td><td>Assign to project</td></tr>
      <tr><td><span class="param-name">heading_uuid</span></td><td class="param-type">string</td><td>Assign to heading within project</td></tr>
      <tr><td><span class="param-name">area_uuid</span></td><td class="param-type">string</td><td>Assign to area</td></tr>
      <tr><td><span class="param-name">tags</span></td><td class="param-type">string</td><td>Comma-separated tag UUIDs</td></tr>
      <tr><td><span class="param-name">checklist</span></td><td class="param-type">string</td><td>Comma-separated checklist items</td></tr>
      <tr><td><span class="param-name">reminder_date</span></td><td class="param-type">string</td><td>YYYY-MM-DD (use with reminder_time)</td></tr>
      <tr><td><span class="param-name">reminder_time</span></td><td class="param-type">string</td><td>HH:MM 24h (use with reminder_date)</td></tr>
      <tr><td><span class="param-name">recurrence</span></td><td class="param-type">string</td><td>daily, weekly, weekly:mon,wed, monthly, monthly:15, monthly:last, yearly, every N days/weeks</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">create_project</div>
    <div class="tool-entry-desc">Create a project</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">title</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Project title</td></tr>
      <tr><td><span class="param-name">note</span></td><td class="param-type">string</td><td>Project notes</td></tr>
      <tr><td><span class="param-name">schedule</span></td><td class="param-type">enum</td><td>today, anytime, someday, inbox</td></tr>
      <tr><td><span class="param-name">deadline</span></td><td class="param-type">string</td><td>YYYY-MM-DD</td></tr>
      <tr><td><span class="param-name">scheduled</span></td><td class="param-type">string</td><td>YYYY-MM-DD</td></tr>
      <tr><td><span class="param-name">area_uuid</span></td><td class="param-type">string</td><td>Assign to area</td></tr>
      <tr><td><span class="param-name">tags</span></td><td class="param-type">string</td><td>Comma-separated tag UUIDs</td></tr>
      <tr><td><span class="param-name">recurrence</span></td><td class="param-type">string</td><td>daily, weekly, weekly:mon,wed, monthly, monthly:15, monthly:last, yearly, every N days/weeks</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">create_heading</div>
    <div class="tool-entry-desc">Create a heading in a project</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">title</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Heading title</td></tr>
      <tr><td><span class="param-name">project_uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Parent project UUID</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">create_area</div>
    <div class="tool-entry-desc">Create an area</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">name</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Area name</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">create_tag</div>
    <div class="tool-entry-desc">Create a tag</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">name</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Tag name</td></tr>
      <tr><td><span class="param-name">shorthand</span></td><td class="param-type">string</td><td>Short name for the tag</td></tr>
      <tr><td><span class="param-name">parent_uuid</span></td><td class="param-type">string</td><td>Parent tag UUID for nesting</td></tr>
    </table>
  </div>
</div>

<!-- Modify Tools -->
<div class="docs-section">
  <div class="category-header">
    <span class="category-dot modify"></span>
    <h3>Modify</h3>
    <span class="count">7 tools</span>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">edit_item</div>
    <div class="tool-entry-desc">Edit a task or project (only provided fields change)</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Item UUID</td></tr>
      <tr><td><span class="param-name">title</span></td><td class="param-type">string</td><td>New title</td></tr>
      <tr><td><span class="param-name">note</span></td><td class="param-type">string</td><td>New notes</td></tr>
      <tr><td><span class="param-name">schedule</span></td><td class="param-type">enum</td><td>today, anytime, someday, inbox</td></tr>
      <tr><td><span class="param-name">deadline</span></td><td class="param-type">string</td><td>YYYY-MM-DD</td></tr>
      <tr><td><span class="param-name">scheduled</span></td><td class="param-type">string</td><td>YYYY-MM-DD</td></tr>
      <tr><td><span class="param-name">area_uuid</span></td><td class="param-type">string</td><td>Move to area</td></tr>
      <tr><td><span class="param-name">project_uuid</span></td><td class="param-type">string</td><td>Move to project</td></tr>
      <tr><td><span class="param-name">heading_uuid</span></td><td class="param-type">string</td><td>Move to heading</td></tr>
      <tr><td><span class="param-name">tags</span></td><td class="param-type">string</td><td>Comma-separated tag UUIDs</td></tr>
      <tr><td><span class="param-name">recurrence</span></td><td class="param-type">string</td><td>daily, weekly, monthly, yearly, etc. Use "none" to clear.</td></tr>
      <tr><td><span class="param-name">status</span></td><td class="param-type">enum</td><td>pending, completed, canceled</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">edit_area</div>
    <div class="tool-entry-desc">Rename an area</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Area UUID</td></tr>
      <tr><td><span class="param-name">name</span><span class="param-required">required</span></td><td class="param-type">string</td><td>New area name</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">edit_tag</div>
    <div class="tool-entry-desc">Edit a tag (name, shorthand, or parent)</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Tag UUID</td></tr>
      <tr><td><span class="param-name">name</span></td><td class="param-type">string</td><td>New tag name</td></tr>
      <tr><td><span class="param-name">shorthand</span></td><td class="param-type">string</td><td>New shorthand/abbreviation</td></tr>
      <tr><td><span class="param-name">parent_uuid</span></td><td class="param-type">string</td><td>New parent tag UUID</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">trash_item</div>
    <div class="tool-entry-desc">Move an item to trash</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Item UUID</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">restore_item</div>
    <div class="tool-entry-desc">Restore an item from trash</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Item UUID</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">delete_area</div>
    <div class="tool-entry-desc">Permanently delete an area</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Area UUID</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">delete_tag</div>
    <div class="tool-entry-desc">Permanently delete a tag</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Tag UUID</td></tr>
    </table>
  </div>
</div>

<!-- Checklist Tools -->
<div class="docs-section">
  <div class="category-header">
    <span class="category-dot modify"></span>
    <h3>Checklist</h3>
    <span class="count">4 tools</span>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">add_checklist_item</div>
    <div class="tool-entry-desc">Add a checklist item to a task</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">task_uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Parent task UUID</td></tr>
      <tr><td><span class="param-name">title</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Checklist item title</td></tr>
      <tr><td><span class="param-name">index</span></td><td class="param-type">number</td><td>Sort position (default 0)</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">edit_checklist_item</div>
    <div class="tool-entry-desc">Edit a checklist item (only provided fields change)</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Checklist item UUID</td></tr>
      <tr><td><span class="param-name">title</span></td><td class="param-type">string</td><td>New title</td></tr>
      <tr><td><span class="param-name">index</span></td><td class="param-type">number</td><td>New sort position</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">complete_checklist_item</div>
    <div class="tool-entry-desc">Complete or uncomplete a checklist item</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Checklist item UUID</td></tr>
      <tr><td><span class="param-name">uncomplete</span></td><td class="param-type">bool</td><td>Set true to mark as pending instead (default false)</td></tr>
    </table>
  </div>

  <div class="tool-entry">
    <div class="tool-entry-name">delete_checklist_item</div>
    <div class="tool-entry-desc">Delete a checklist item</div>
    <table class="params-table">
      <tr><th>Parameter</th><th>Type</th><th>Description</th></tr>
      <tr><td><span class="param-name">uuid</span><span class="param-required">required</span></td><td class="param-type">string</td><td>Checklist item UUID</td></tr>
    </table>
  </div>
</div>

<!-- Output Format -->
<div class="output-section">
  <h3>Output Format</h3>
  <p>Tasks are returned in the following JSON shape:</p>
  <pre>{
  "uuid": "...",
  "title": "...",
  "note": "...",
  "status": "pending | completed | canceled",
  "schedule": "inbox | today | anytime | someday | upcoming",
  "scheduledDate": "YYYY-MM-DD",
  "deadlineDate": "YYYY-MM-DD",
  "creationDate": "YYYY-MM-DDTHH:MM:SSZ",
  "modificationDate": "YYYY-MM-DDTHH:MM:SSZ",
  "completionDate": "YYYY-MM-DDTHH:MM:SSZ",
  "areas": [{"uuid": "...", "name": "..."}],
  "project": {"uuid": "...", "name": "..."},
  "tags": [{"uuid": "...", "name": "..."}]
}</pre>
</div>

<!-- Get Started -->
<div class="docs-section">
  <div class="section-header">
    <div class="section-label">Get Started</div>
    <div class="section-title">Setup MCP with <span class="client-selector-wrapper"><span class="client-selector-trigger" id="docsClientTrigger"><span id="docsClientLabel">Claude.ai</span> &#9662;</span><div class="client-dropdown" id="docsClientDropdown"><div class="client-dropdown-item active" data-client="claude-ai"><svg width="16" height="16" viewBox="0 0 24 24" fill="#d97757" xmlns="http://www.w3.org/2000/svg"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg> Claude.ai</div><div class="client-dropdown-item" data-client="claude-code"><svg width="16" height="16" viewBox="0 0 24 24" fill="#d97757" xmlns="http://www.w3.org/2000/svg"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg> Claude Code</div><div class="client-dropdown-item" data-client="chatgpt"><svg width="16" height="16" viewBox="0 0 16 16" fill="#74AA9C" xmlns="http://www.w3.org/2000/svg"><path d="M14.949 6.547a3.94 3.94 0 0 0-.348-3.273 4.11 4.11 0 0 0-4.4-1.934A4.1 4.1 0 0 0 8.423.2 4.15 4.15 0 0 0 6.305.086a4.1 4.1 0 0 0-1.891.948 4.04 4.04 0 0 0-1.158 1.753 4.1 4.1 0 0 0-1.563.679A4 4 0 0 0 .554 4.72a3.99 3.99 0 0 0 .502 4.731 3.94 3.94 0 0 0 .346 3.274 4.11 4.11 0 0 0 4.402 1.933c.382.425.852.764 1.377.995.526.231 1.095.35 1.67.346 1.78.002 3.358-1.132 3.901-2.804a4.1 4.1 0 0 0 1.563-.68 4 4 0 0 0 1.14-1.253 3.99 3.99 0 0 0-.506-4.716m-6.097 8.406a3.05 3.05 0 0 1-1.945-.694l.096-.054 3.23-1.838a.53.53 0 0 0 .265-.455v-4.49l1.366.778q.02.011.025.035v3.722c-.003 1.653-1.361 2.992-3.037 2.996m-6.53-2.75a2.95 2.95 0 0 1-.36-2.01l.095.057L5.29 12.09a.53.53 0 0 0 .527 0l3.949-2.246v1.555a.05.05 0 0 1-.022.041L6.473 13.3c-1.454.826-3.311.335-4.15-1.098m-.85-6.94A3.02 3.02 0 0 1 3.07 3.949v3.785a.51.51 0 0 0 .262.451l3.93 2.237-1.366.779a.05.05 0 0 1-.048 0L2.585 9.342a2.98 2.98 0 0 1-1.113-4.094zm11.216 2.571L8.747 5.576l1.362-.776a.05.05 0 0 1 .048 0l3.265 1.86a3 3 0 0 1 1.173 1.207 2.96 2.96 0 0 1-.27 3.2 3.05 3.05 0 0 1-1.36.997V8.279a.52.52 0 0 0-.276-.445m1.36-2.015-.097-.057-3.226-1.855a.53.53 0 0 0-.53 0L6.249 6.153V4.598a.04.04 0 0 1 .019-.04L9.533 2.7a3.07 3.07 0 0 1 3.257.139c.474.325.843.778 1.066 1.303.223.526.289 1.103.191 1.664zM5.503 8.575 4.139 7.8a.05.05 0 0 1-.026-.037V4.049c0-.57.166-1.127.476-1.607s.752-.864 1.275-1.105a3.08 3.08 0 0 1 3.234.41l-.096.054-3.23 1.838a.53.53 0 0 0-.265.455zm.742-1.577 1.758-1 1.762 1v2l-1.755 1-1.762-1z"/></svg> ChatGPT</div><div class="client-dropdown-item" data-client="cursor"><svg width="16" height="16" viewBox="0 0 24 24" fill="#F54E00" xmlns="http://www.w3.org/2000/svg"><path d="M11.503.131 1.891 5.678a.84.84 0 0 0-.42.726v11.188c0 .3.162.575.42.724l9.609 5.55a1 1 0 0 0 .998 0l9.61-5.55a.84.84 0 0 0 .42-.724V6.404a.84.84 0 0 0-.42-.726L12.497.131a1.01 1.01 0 0 0-.996 0M2.657 6.338h18.55c.263 0 .43.287.297.515L12.23 22.918c-.062.107-.229.064-.229-.06V12.335a.59.59 0 0 0-.295-.51l-9.11-5.257c-.109-.063-.064-.23.061-.23"/></svg> Cursor</div><div class="client-dropdown-item" data-client="windsurf"><svg width="16" height="16" viewBox="0 0 24 24" fill="#06B6D4" xmlns="http://www.w3.org/2000/svg"><path d="M23.55 5.067c-1.2038-.002-2.1806.973-2.1806 2.1765v4.8676c0 .972-.8035 1.7594-1.7597 1.7594-.568 0-1.1352-.286-1.4718-.7659l-4.9713-7.1003c-.4125-.5896-1.0837-.941-1.8103-.941-1.1334 0-2.1533.9635-2.1533 2.153v4.8957c0 .972-.7969 1.7594-1.7596 1.7594-.57 0-1.1363-.286-1.4728-.7658L.4076 5.1598C.2822 4.9798 0 5.0688 0 5.2882v4.2452c0 .2147.0656.4228.1884.599l5.4748 7.8183c.3234.462.8006.8052 1.3509.9298 1.3771.313 2.6446-.747 2.6446-2.0977v-4.893c0-.972.7875-1.7593 1.7596-1.7593h.003a1.798 1.798 0 0 1 1.4718.7658l4.9723 7.0994c.4135.5905 1.05.941 1.8093.941 1.1587 0 2.1515-.9645 2.1515-2.153v-4.8948c0-.972.7875-1.7594 1.7596-1.7594h.194a.22.22 0 0 0 .2204-.2202v-4.622a.22.22 0 0 0-.2203-.2203Z"/></svg> Windsurf</div></div></span></div>
  </div>

  <div class="client-instructions-container" id="docsInstructionsContainer">

    <div class="client-instructions active" data-client="claude-ai">
      <div class="step"><span class="num">1</span><div class="step-text">Go to <strong>Settings &rarr; Connectors &rarr; Add custom connector</strong></div></div>
      <div class="step"><span class="num">2</span><div class="step-text">Enter name: <strong>Things Cloud</strong></div></div>
      <div class="step"><span class="num">3</span><div class="step-text">Enter URL: <strong><span class="mcp-url"></span></strong></div></div>
      <div class="step"><span class="num">4</span><div class="step-text">Click <strong>Add</strong>, then enable in chat via the &ldquo;+&rdquo; button</div></div>
    </div>

    <div class="client-instructions" data-client="claude-code">
      <div class="step"><span class="num">1</span><div class="step-text">Run the following command:</div></div>
      <pre><code>claude mcp add --transport http \
  --header "Authorization: Basic BASE64_ENCODE(email:password)" \
  things-cloud <span class="mcp-url"></span></code></pre>
      <div class="note">Replace <strong>BASE64_ENCODE(email:password)</strong> with your base64-encoded Things Cloud credentials (email:password). Generate with: <code>echo -n 'email:password' | base64</code></div>
      <div class="step"><span class="num">2</span><div class="step-text">Verify with the <strong>/mcp</strong> command inside Claude Code.</div></div>
    </div>

    <div class="client-instructions" data-client="chatgpt">
      <div class="step"><span class="num">1</span><div class="step-text">Go to <strong>Settings &rarr; Apps &amp; Connectors &rarr; Advanced</strong>, enable <strong>Developer Mode</strong></div></div>
      <div class="step"><span class="num">2</span><div class="step-text">Click <strong>Add Connector</strong></div></div>
      <div class="step"><span class="num">3</span><div class="step-text">Enter name: <strong>Things Cloud</strong>, URL: <strong><span class="mcp-url"></span></strong></div></div>
      <div class="step"><span class="num">4</span><div class="step-text">In a new chat, click &ldquo;+&rdquo; to select the connector</div></div>
      <div class="note">Note: ChatGPT requires a publicly accessible URL (use ngrok for local dev).</div>
    </div>

    <div class="client-instructions" data-client="cursor">
      <div class="step"><span class="num">1</span><div class="step-text">Add to <strong>~/.cursor/mcp.json</strong>:</div></div>
      <pre><code>{
  "mcpServers": {
    "things-cloud": {
      "url": "<span class="mcp-url"></span>",
      "headers": {
        "Authorization": "Basic BASE64_ENCODE(email:password)"
      }
    }
  }
}</code></pre>
      <div class="note">Replace <strong>BASE64_ENCODE(email:password)</strong> with your base64-encoded Things Cloud credentials (email:password). Generate with: <code>echo -n 'email:password' | base64</code></div>
    </div>

    <div class="client-instructions" data-client="windsurf">
      <div class="step"><span class="num">1</span><div class="step-text">Add to <strong>~/.codeium/windsurf/mcp_config.json</strong>:</div></div>
      <pre><code>{
  "mcpServers": {
    "things-cloud": {
      "serverUrl": "<span class="mcp-url"></span>",
      "headers": {
        "Authorization": "Basic BASE64_ENCODE(email:password)"
      }
    }
  }
}</code></pre>
      <div class="note">Replace <strong>BASE64_ENCODE(email:password)</strong> with your base64-encoded Things Cloud credentials (email:password). Generate with: <code>echo -n 'email:password' | base64</code></div>
    </div>

  </div>
</div>

</div><!-- /container -->

<!-- Footer -->
<footer>
  <div class="container">
    <p>Powered by <a href="https://github.com/arthursoares/things-cloud-sdk" target="_blank" rel="noopener">Things Cloud SDK</a></p>
  </div>
</footer>

<script>
(function(){
  var mcpUrl = window.location.origin + "/mcp";
  document.querySelectorAll(".mcp-url").forEach(function(el){ el.textContent = mcpUrl; });
})();
(function(){
  var iconMap = {
    "claude-ai": '<svg width="16" height="16" viewBox="0 0 24 24" fill="#d97757" xmlns="http://www.w3.org/2000/svg"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg>',
    "claude-code": '<svg width="16" height="16" viewBox="0 0 24 24" fill="#d97757" xmlns="http://www.w3.org/2000/svg"><path d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z"/></svg>',
    "chatgpt": '<svg width="16" height="16" viewBox="0 0 16 16" fill="#74AA9C" xmlns="http://www.w3.org/2000/svg"><path d="M14.949 6.547a3.94 3.94 0 0 0-.348-3.273 4.11 4.11 0 0 0-4.4-1.934A4.1 4.1 0 0 0 8.423.2 4.15 4.15 0 0 0 6.305.086a4.1 4.1 0 0 0-1.891.948 4.04 4.04 0 0 0-1.158 1.753 4.1 4.1 0 0 0-1.563.679A4 4 0 0 0 .554 4.72a3.99 3.99 0 0 0 .502 4.731 3.94 3.94 0 0 0 .346 3.274 4.11 4.11 0 0 0 4.402 1.933c.382.425.852.764 1.377.995.526.231 1.095.35 1.67.346 1.78.002 3.358-1.132 3.901-2.804a4.1 4.1 0 0 0 1.563-.68 4 4 0 0 0 1.14-1.253 3.99 3.99 0 0 0-.506-4.716m-6.097 8.406a3.05 3.05 0 0 1-1.945-.694l.096-.054 3.23-1.838a.53.53 0 0 0 .265-.455v-4.49l1.366.778q.02.011.025.035v3.722c-.003 1.653-1.361 2.992-3.037 2.996m-6.53-2.75a2.95 2.95 0 0 1-.36-2.01l.095.057L5.29 12.09a.53.53 0 0 0 .527 0l3.949-2.246v1.555a.05.05 0 0 1-.022.041L6.473 13.3c-1.454.826-3.311.335-4.15-1.098m-.85-6.94A3.02 3.02 0 0 1 3.07 3.949v3.785a.51.51 0 0 0 .262.451l3.93 2.237-1.366.779a.05.05 0 0 1-.048 0L2.585 9.342a2.98 2.98 0 0 1-1.113-4.094zm11.216 2.571L8.747 5.576l1.362-.776a.05.05 0 0 1 .048 0l3.265 1.86a3 3 0 0 1 1.173 1.207 2.96 2.96 0 0 1-.27 3.2 3.05 3.05 0 0 1-1.36.997V8.279a.52.52 0 0 0-.276-.445m1.36-2.015-.097-.057-3.226-1.855a.53.53 0 0 0-.53 0L6.249 6.153V4.598a.04.04 0 0 1 .019-.04L9.533 2.7a3.07 3.07 0 0 1 3.257.139c.474.325.843.778 1.066 1.303.223.526.289 1.103.191 1.664zM5.503 8.575 4.139 7.8a.05.05 0 0 1-.026-.037V4.049c0-.57.166-1.127.476-1.607s.752-.864 1.275-1.105a3.08 3.08 0 0 1 3.234.41l-.096.054-3.23 1.838a.53.53 0 0 0-.265.455zm.742-1.577 1.758-1 1.762 1v2l-1.755 1-1.762-1z"/></svg>',
    "cursor": '<svg width="16" height="16" viewBox="0 0 24 24" fill="#F54E00" xmlns="http://www.w3.org/2000/svg"><path d="M11.503.131 1.891 5.678a.84.84 0 0 0-.42.726v11.188c0 .3.162.575.42.724l9.609 5.55a1 1 0 0 0 .998 0l9.61-5.55a.84.84 0 0 0 .42-.724V6.404a.84.84 0 0 0-.42-.726L12.497.131a1.01 1.01 0 0 0-.996 0M2.657 6.338h18.55c.263 0 .43.287.297.515L12.23 22.918c-.062.107-.229.064-.229-.06V12.335a.59.59 0 0 0-.295-.51l-9.11-5.257c-.109-.063-.064-.23.061-.23"/></svg>',
    "windsurf": '<svg width="16" height="16" viewBox="0 0 24 24" fill="#06B6D4" xmlns="http://www.w3.org/2000/svg"><path d="M23.55 5.067c-1.2038-.002-2.1806.973-2.1806 2.1765v4.8676c0 .972-.8035 1.7594-1.7597 1.7594-.568 0-1.1352-.286-1.4718-.7659l-4.9713-7.1003c-.4125-.5896-1.0837-.941-1.8103-.941-1.1334 0-2.1533.9635-2.1533 2.153v4.8957c0 .972-.7969 1.7594-1.7596 1.7594-.57 0-1.1363-.286-1.4728-.7658L.4076 5.1598C.2822 4.9798 0 5.0688 0 5.2882v4.2452c0 .2147.0656.4228.1884.599l5.4748 7.8183c.3234.462.8006.8052 1.3509.9298 1.3771.313 2.6446-.747 2.6446-2.0977v-4.893c0-.972.7875-1.7593 1.7596-1.7593h.003a1.798 1.798 0 0 1 1.4718.7658l4.9723 7.0994c.4135.5905 1.05.941 1.8093.941 1.1587 0 2.1515-.9645 2.1515-2.153v-4.8948c0-.972.7875-1.7594 1.7596-1.7594h.194a.22.22 0 0 0 .2204-.2202v-4.622a.22.22 0 0 0-.2203-.2203Z"/></svg>'
  };
  var nameMap = {
    "claude-ai": "Claude.ai",
    "claude-code": "Claude Code",
    "chatgpt": "ChatGPT",
    "cursor": "Cursor",
    "windsurf": "Windsurf"
  };

  var trigger = document.getElementById("docsClientTrigger");
  var dropdown = document.getElementById("docsClientDropdown");
  var label = document.getElementById("docsClientLabel");
  var container = document.getElementById("docsInstructionsContainer");
  var items = dropdown.querySelectorAll(".client-dropdown-item");
  var allInstructions = container.querySelectorAll(".client-instructions");

  label.innerHTML = iconMap["claude-ai"] + " " + nameMap["claude-ai"];

  trigger.addEventListener("click", function(e){
    e.stopPropagation();
    dropdown.classList.toggle("open");
  });

  items.forEach(function(item){
    item.addEventListener("click", function(e){
      e.stopPropagation();
      var clientId = this.getAttribute("data-client");
      label.innerHTML = iconMap[clientId] + " " + nameMap[clientId];
      items.forEach(function(el){ el.classList.remove("active"); });
      this.classList.add("active");
      allInstructions.forEach(function(el){
        if(el.getAttribute("data-client") === clientId){
          el.classList.add("active");
        } else {
          el.classList.remove("active");
        }
      });
      dropdown.classList.remove("open");
    });
  });

  document.addEventListener("click", function(){
    dropdown.classList.remove("open");
  });
})();
</script>

</body>
</html>`

func handleLandingPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(LandingPageHTML))
}

func handleDocsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(DocsPageHTML))
}

// HowItWorksPageHTML contains the full HTML "How it Works" page.
var HowItWorksPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="icon" type="image/png" sizes="32x32" href="/favicon.ico">
<link rel="icon" type="image/svg+xml" href="/favicon.svg">
<title>How it Works - Things Cloud MCP</title>
<style>
` + sharedCSS + `

/* -- How it Works page styles -- */
.hiw-nav{
  padding:24px 0 0;
}
.hiw-nav a{
  color:var(--blue);
  text-decoration:none;
  font-size:14px;
  font-weight:500;
}
.hiw-nav a:hover{text-decoration:underline}

.hiw-hero{
  padding:48px 0 40px;
}
.hiw-hero h2{
  font-size:32px;
  font-weight:700;
  letter-spacing:-0.5px;
  margin-bottom:8px;
}
.hiw-hero p{
  font-size:16px;
  color:var(--text-secondary);
  max-width:540px;
  line-height:1.55;
}

.hiw-section{
  padding:0 0 48px;
}
.hiw-section:last-of-type{
  padding-bottom:64px;
}
.hiw-section .section-title{
  margin-bottom:16px;
}
.hiw-section p{
  font-size:15px;
  color:var(--text-secondary);
  line-height:1.7;
  margin-bottom:12px;
  max-width:680px;
}
.hiw-section p:last-child{
  margin-bottom:0;
}
.hiw-section p strong{
  color:var(--text);
  font-weight:600;
}

/* -- Warning box -- */
.warning-box{
  border-left:4px solid #F5A623;
  background:#FFF8EB;
  border-radius:var(--radius-sm);
  padding:20px 24px;
  margin-top:8px;
}
@media(prefers-color-scheme:dark){
  .warning-box{
    background:#3A3520;
    border-left-color:#F5A623;
  }
}
.warning-box .warning-title{
  font-size:15px;
  font-weight:700;
  color:var(--text);
  margin-bottom:10px;
}
.warning-box p{
  font-size:14px;
  color:var(--text-secondary);
  line-height:1.65;
  margin-bottom:8px;
}
.warning-box p:last-child{
  margin-bottom:0;
}

/* -- Architecture diagram -- */
.arch-diagram{
  display:flex;
  flex-direction:column;
  align-items:center;
  gap:0;
  padding:32px 0;
  max-width:420px;
  margin:0 auto;
}
.arch-box{
  width:100%;
  text-align:center;
  padding:16px 20px;
  border:1px solid var(--divider);
  border-radius:var(--radius-sm);
  background:var(--surface);
  font-size:14px;
  font-weight:500;
  color:var(--text);
  line-height:1.4;
}
.arch-box .arch-sub{
  display:block;
  font-size:12px;
  font-weight:400;
  color:var(--text-secondary);
  margin-top:2px;
}
.arch-box.highlight{
  border-color:var(--blue);
  background:var(--bg-alt);
}
.arch-arrow{
  display:flex;
  flex-direction:column;
  align-items:center;
  gap:0;
  padding:6px 0;
  color:var(--text-secondary);
  font-size:12px;
  line-height:1.3;
}
.arch-arrow-icon{
  font-size:18px;
  line-height:1;
  color:var(--text-secondary);
}
.arch-arrow-label{
  font-size:11px;
  font-weight:500;
  color:var(--text-secondary);
  letter-spacing:0.3px;
}

/* -- Capabilities list -- */
.cap-list{
  list-style:none;
  padding:0;
  margin:0;
}
.cap-list li{
  padding:8px 0;
  border-bottom:1px solid var(--divider);
  font-size:14px;
  color:var(--text);
  display:flex;
  align-items:center;
  gap:10px;
}
.cap-list li:last-child{
  border-bottom:none;
}
.cap-dot{
  width:6px;height:6px;
  border-radius:50%;
  background:var(--blue);
  flex-shrink:0;
}
</style>
</head>
<body>

<!-- Header -->
<header>
  <div class="container header-inner">
    <div class="logo">` + cloudCheckSVGSmall + `</div>
    <div class="header-text">
      <h1>Things Cloud MCP</h1>
    </div>
    <div class="header-right">
      <span class="header-status"><span class="dot"></span> Online</span>
      <span class="header-divider">|</span>
      <a class="header-doc-link" href="/how-it-works">How it Works</a>
      <span class="header-divider">|</span>
      <a class="header-doc-link" href="/docs">Documentation</a>
    </div>
  </div>
</header>

<div class="container">

<!-- Back nav -->
<div class="hiw-nav">
  <a href="/">&larr; Back to home</a>
</div>

<!-- Hero -->
<div class="hiw-hero">
  <h2>How it Works</h2>
  <p>Understand how Things Cloud MCP connects your AI to Things 3 from anywhere.</p>
</div>

<!-- The Problem -->
<div class="hiw-section">
  <div class="section-label">The Problem</div>
  <div class="section-title">Local-only MCP servers</div>
  <p>Most MCP servers for Things 3 run locally on your Mac, communicating directly with the Things app via AppleScript or URL schemes.</p>
  <p>This means you can <strong>only manage tasks from the same machine</strong> where Things is running. If you want to use a web-based AI like Claude.ai or ChatGPT, or manage tasks from a Linux server, you are stuck.</p>
</div>

<!-- Our Approach -->
<div class="hiw-section">
  <div class="section-label">Our Approach</div>
  <div class="section-title">Talk directly to Things Cloud</div>
  <p>Things Cloud MCP takes a different approach: it talks directly to <strong>Things Cloud</strong> &mdash; the sync service that keeps your tasks in sync across all your Apple devices.</p>
  <p>We use a reverse-engineered Things Cloud SDK to read and write your tasks over the internet. This means the MCP server can run <strong>anywhere</strong> &mdash; on a remote server, in a Docker container, on a Linux machine &mdash; and any MCP-compatible AI client can connect to it.</p>
  <p>The server implements the <strong>Streamable HTTP</strong> transport, so web-based clients like Claude.ai and ChatGPT can connect directly.</p>
</div>

<!-- Architecture -->
<div class="hiw-section">
  <div class="section-label">Architecture</div>
  <div class="section-title">End-to-end flow</div>

  <div class="arch-diagram">
    <div class="arch-box">Your AI Client<span class="arch-sub">Claude.ai, ChatGPT, Claude Code, Cursor</span></div>
    <div class="arch-arrow"><span class="arch-arrow-icon">&darr;</span><span class="arch-arrow-label">MCP over HTTP</span></div>
    <div class="arch-box highlight">Things Cloud MCP Server<span class="arch-sub">This project</span></div>
    <div class="arch-arrow"><span class="arch-arrow-icon">&darr;</span><span class="arch-arrow-label">Things Cloud API</span></div>
    <div class="arch-box">Things Cloud<span class="arch-sub">Apple sync service</span></div>
    <div class="arch-arrow"><span class="arch-arrow-icon">&darr;</span><span class="arch-arrow-label">iCloud Sync</span></div>
    <div class="arch-box">Things 3<span class="arch-sub">iPhone / iPad / Mac</span></div>
  </div>
</div>

<!-- Warning -->
<div class="hiw-section">
  <div class="section-label">Important</div>
  <div class="section-title">Back up your data first</div>
  <div class="warning-box">
    <div class="warning-title">&#9888;&#65039; Unofficial API</div>
    <p>This project uses a reverse-engineered, unofficial API to communicate with Things Cloud. While we have tested it thoroughly, there is always a risk when using unofficial APIs.</p>
    <p><strong>Before connecting:</strong> Make sure your Things 3 data is backed up. Things automatically syncs to iCloud, but you may also want to export your data via File &rarr; Export in Things for Mac.</p>
    <p>The authors are not responsible for any data loss.</p>
  </div>
</div>

<!-- What you can do -->
<div class="hiw-section">
  <div class="section-label">Capabilities</div>
  <div class="section-title">What you can do</div>
  <ul class="cap-list">
    <li><span class="cap-dot"></span> Read tasks, projects, areas, and tags</li>
    <li><span class="cap-dot"></span> Create new tasks, projects, headings, areas, and tags</li>
    <li><span class="cap-dot"></span> Edit tasks and projects &mdash; title, notes, schedule, deadline, tags, status</li>
    <li><span class="cap-dot"></span> Move items to trash</li>
    <li><span class="cap-dot"></span> All changes sync to your Things apps in real-time via Things Cloud</li>
  </ul>
</div>

</div><!-- /container -->

<!-- Footer -->
<footer>
  <div class="container">
    <p>Powered by <a href="https://github.com/arthursoares/things-cloud-sdk" target="_blank" rel="noopener">Things Cloud SDK</a></p>
  </div>
</footer>

</body>
</html>`

func handleHowItWorksPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(HowItWorksPageHTML))
}

// faviconSVG is a standalone SVG used as the favicon (cloud with checkmark).
const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="64" height="64" fill="none">
  <path d="M50 46a11 11 0 0 0 0-22 11 11 0 0 0-1-.04 15 15 0 0 0-29-2A13 13 0 0 0 14 46h36z" fill="#1A7CF9"/>
  <polyline points="24,34 30,40 42,28" stroke="#fff" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
</svg>`

// faviconPNG is a base64-encoded 32x32 PNG of a blue cloud with white checkmark.
const faviconPNG = "iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAABHUlEQVR4nGJiGGAw6oABdwALOZqkan7+xyb+rIWdkVSzSNKAy2JKHEJ0FBBrOalqiXIAKQaSqmfAEyFBB5Dje1L04nUAuZZXuzEzPG1mA+Oph/7gNQNnaqXE8ixbZhQx6dpfOHMG1hCgpuWEzMRwALUtn3b4L16zmWhteeuuvwz47CCYC5ATFIhNieXYAEEHIBsOYiM7glLLGcipjNAdRInlDMSEAHIiQraYGpYT5QCQgdgcQQ3LMRyAq7Ag5AhSLEe3g+jKCJcjyPU5yQ6AOeL8439wPohNieUM5OQCn1l/KLIQHWCEADntOmIBNrMHZ4OEFqGAy0yCFlHSIsJnMdEOINchxIYi0WmAlGghRS1ZcU3NntGAA0AAAAD//8S3oe5VGX2KAAAAAElFTkSuQmCC"

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=86400")

	if strings.HasSuffix(r.URL.Path, ".svg") {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write([]byte(faviconSVG))
		return
	}

	// For /favicon.ico and any .png request, serve the PNG version.
	pngData, err := base64.StdEncoding.DecodeString(faviconPNG)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(pngData)
}
