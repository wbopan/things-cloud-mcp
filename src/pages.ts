// Sign-in and success pages for the OAuth flow (unchanged from the Go server).

const CSS = `*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}

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
.header-text{flex:1}
.github-link{
  display:flex;align-items:center;justify-content:center;
  color:var(--text-secondary);
  transition:color 0.15s;
}
.github-link:hover{color:var(--text)}
.github-link svg{width:24px;height:24px}

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
`;

export const LOGIN_PAGE = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Sign In - Things Cloud MCP</title>
<style>
${CSS}
.auth-container{
  min-height:100vh;
  display:flex;
  align-items:center;
  justify-content:center;
  padding:24px;
}
.auth-card{
  width:100%;
  max-width:380px;
  background:var(--surface);
  border:1px solid var(--divider);
  border-radius:var(--radius);
  padding:40px 32px;
  box-shadow:0 2px 12px rgba(0,0,0,0.06);
}
@media(prefers-color-scheme:dark){
  .auth-card{
    box-shadow:0 2px 12px rgba(0,0,0,0.2);
  }
}
.auth-icon{
  display:flex;
  justify-content:center;
  margin-bottom:24px;
}
.auth-icon svg{width:48px;height:48px}
.auth-title{
  font-size:20px;
  font-weight:700;
  text-align:center;
  letter-spacing:-0.3px;
  margin-bottom:6px;
}
.auth-subtitle{
  font-size:14px;
  color:var(--text-secondary);
  text-align:center;
  margin-bottom:28px;
}
.auth-error{
  background:#FFF2F2;
  color:#D70015;
  border:1px solid #FFD6D6;
  border-radius:var(--radius-sm);
  padding:10px 14px;
  font-size:13px;
  margin-bottom:20px;
  line-height:1.45;
}
@media(prefers-color-scheme:dark){
  .auth-error{
    background:#3A1C1C;
    color:#FF6B6B;
    border-color:#5A2C2C;
  }
}
.auth-field{
  margin-bottom:16px;
}
.auth-field label{
  display:block;
  font-size:13px;
  font-weight:600;
  margin-bottom:6px;
  color:var(--text);
}
.auth-field input{
  width:100%;
  padding:10px 14px;
  font-size:15px;
  border:1px solid var(--divider);
  border-radius:var(--radius-sm);
  background:var(--bg);
  color:var(--text);
  font-family:inherit;
  outline:none;
  transition:border-color 0.15s;
  box-sizing:border-box;
}
.auth-field input:focus{
  border-color:var(--blue);
}
.auth-btn{
  width:100%;
  padding:12px;
  font-size:15px;
  font-weight:600;
  color:#fff;
  background:var(--blue);
  border:none;
  border-radius:var(--radius-sm);
  cursor:pointer;
  font-family:inherit;
  transition:background 0.15s;
  margin-top:8px;
}
.auth-btn:hover{
  background:var(--blue-hover);
}
</style>
</head>
<body>
<div class="auth-container">
  <div class="auth-card">
    <div class="auth-icon"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="48" height="48" fill="none">
  <path d="M50 46a11 11 0 0 0 0-22 11 11 0 0 0-1-.04 15 15 0 0 0-29-2A13 13 0 0 0 14 46h36z" fill="#1A7CF9"/>
  <polyline points="24,34 30,40 42,28" stroke="#fff" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
</svg></div>
    <div class="auth-title">Sign in with Things Cloud</div>
    <div class="auth-subtitle">{{subtitle}}</div>
    {{error}}
    <form method="POST" action="/authorize?{{query}}">
      <div class="auth-field">
        <label for="email">Email</label>
        <input type="email" id="email" name="email" required autocomplete="email" autofocus>
      </div>
      <div class="auth-field">
        <label for="password">Password</label>
        <input type="password" id="password" name="password" required autocomplete="current-password">
      </div>
      <button type="submit" class="auth-btn">Authorize</button>
    </form>
    <p style="margin-top:20px;font-size:11px;color:var(--text-secondary);text-align:center;line-height:1.5">Not affiliated with Cultured Code. Built through reverse engineering with <a href="https://github.com/arthursoares/things-cloud-sdk" target="_blank" rel="noopener" style="color:var(--blue);text-decoration:none">Things Cloud SDK</a>.</p>
  </div>
</div>
</body>
</html>`;

export const SUCCESS_PAGE = `<!DOCTYPE html><html><head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Authorized - Things Cloud MCP</title>
<style>
${CSS}
.auth-container{
  min-height:100vh;
  display:flex;
  align-items:center;
  justify-content:center;
  padding:24px;
}
.auth-card{
  width:100%;
  max-width:380px;
  background:var(--surface);
  border:1px solid var(--divider);
  border-radius:var(--radius);
  padding:40px 32px;
  box-shadow:0 2px 12px rgba(0,0,0,0.06);
  text-align:center;
}
.auth-icon{
  display:flex;
  justify-content:center;
  margin-bottom:24px;
}
.auth-title{
  font-size:20px;
  font-weight:700;
  letter-spacing:-0.3px;
  margin-bottom:8px;
}
.auth-subtitle{
  font-size:14px;
  color:var(--text-secondary);
  line-height:1.5;
}
@keyframes spin{to{transform:rotate(360deg)}}
.spinner{
  width:20px;height:20px;
  border:2px solid var(--divider);
  border-top-color:var(--blue);
  border-radius:50%;
  animation:spin 0.8s linear infinite;
  display:inline-block;
  vertical-align:middle;
  margin-right:8px;
}
</style>
</head>
<body>
<div class="auth-container">
  <div class="auth-card">
    <div class="auth-icon"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="48" height="48" fill="none">
  <circle cx="32" cy="32" r="28" fill="#34C759"/>
  <polyline points="20,33 28,41 44,25" stroke="#fff" stroke-width="4" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
</svg></div>
    <div class="auth-title">Authorization Successful</div>
    <div class="auth-subtitle">You can close this window.</div>
  </div>
</div>
<script>setTimeout(function(){window.location.href="{{redirect_url}}"},1500);</script>
</body></html>`;
