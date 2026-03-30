package main

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
)

const guestPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Join Call — Wally Conference</title>
<script>
(function() {
  var t = localStorage.getItem('wally-theme') || 'dark';
  if (t === 'auto') {
    t = window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
  }
  document.documentElement.setAttribute('data-theme', t);
})();
</script>
<style>
  /* ── Theme variables ── */
  [data-theme="dark"] {
    --bg: #1a1a2e; --card: #16213e; --input: #0f3460;
    --accent: #e94560; --accent-hover: #c73e54;
    --text: #eee; --text-muted: #888; --success: #4ec94e;
    --border: #333; --toolbar: #16213e;
    --tile: #16213e; --tile-no-video: #0f3460;
    --tb-btn-bg: #2a2a4a; --tb-btn-hover: #3a3a5a;
    --dbg-bg: rgba(0,0,0,.92); --dbg-text: #0f0;
    --dbg-input-bg: #111; --dbg-btn-bg: #333; --dbg-btn-hover: #555;
    --dbg-border: #222; --dbg-btn-border: #555;
    --label-bg: rgba(0,0,0,.6);
  }
  [data-theme="light"] {
    --bg: #f0f2f5; --card: #ffffff; --input: #f8f9fa;
    --accent: #e94560; --accent-hover: #c73e54;
    --text: #1a1a2e; --text-muted: #666; --success: #2d8a2d;
    --border: #ddd; --toolbar: #ffffff;
    --tile: #f0f2f5; --tile-no-video: #e8ecf0;
    --tb-btn-bg: #e0e3e8; --tb-btn-hover: #cdd1d8;
    --dbg-bg: rgba(255,255,255,.95); --dbg-text: #1a7a1a;
    --dbg-input-bg: #f0f2f5; --dbg-btn-bg: #e0e3e8; --dbg-btn-hover: #cdd1d8;
    --dbg-border: #ddd; --dbg-btn-border: #bbb;
    --label-bg: rgba(255,255,255,.7);
  }

  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: var(--bg); color: var(--text);
    display: flex; align-items: center; justify-content: center;
    min-height: 100vh; padding: 1rem;
    overflow: hidden;
  }

  /* ── Join card ── */
  .card {
    position: relative;
    background: var(--card); border-radius: 12px; padding: 2rem;
    max-width: 400px; width: 100%%; box-shadow: 0 4px 24px rgba(0,0,0,.4);
  }
  .wally-logo {
    display: flex; align-items: center; gap: 10px; margin-bottom: .25rem;
  }
  .wally-logo svg { width: 36px; height: 36px; flex-shrink: 0; }
  .wally-logo span { font-size: 1.4rem; font-weight: 700; }
  .subtitle { color: var(--text-muted); font-size: .85rem; margin-bottom: 1.5rem; word-break: break-all; }
  label { display: block; font-size: .9rem; margin-bottom: .4rem; color: var(--text-muted); }
  input {
    width: 100%%; padding: .65rem .8rem; border-radius: 8px;
    border: 1px solid var(--border); background: var(--input); color: var(--text);
    font-size: 1rem; margin-bottom: 1rem; outline: none;
  }
  input:focus { border-color: var(--accent); }
  .join-btn {
    width: 100%%; padding: .7rem; border-radius: 8px; border: none;
    background: var(--accent); color: #fff; font-size: 1rem; font-weight: 600;
    cursor: pointer; transition: background .2s;
  }
  .join-btn:hover { background: var(--accent-hover); }
  .join-btn:disabled { background: #555; cursor: not-allowed; }
  .error { color: var(--accent); font-size: .85rem; margin-top: .5rem; }
  .success { color: var(--success); font-size: .85rem; margin-top: .5rem; }
  .branding { text-align: center; margin-top: 1.5rem; color: var(--text-muted); font-size: .75rem; }
  .branding a { color: var(--text-muted); text-decoration: none; }

  /* ── Theme picker ── */
  .theme-picker {
    position: absolute; top: 12px; right: 12px;
    display: flex; gap: 4px;
  }
  .theme-picker button {
    background: none; border: 1px solid var(--border);
    color: var(--text-muted); border-radius: 6px;
    width: 28px; height: 28px; cursor: pointer;
    font-size: 14px; display: flex; align-items: center; justify-content: center;
  }
  .theme-picker button.active { color: var(--accent); border-color: var(--accent); }

  /* ── Call view ── */
  #callView {
    display: none; position: fixed; inset: 0;
    background: var(--bg); flex-direction: column;
  }
  #callView.active { display: flex; }

  #videoGrid {
    flex: 1; display: grid; gap: 4px; padding: 4px;
    grid-template-columns: 1fr;
    overflow: hidden;
  }

  .tile {
    position: relative; background: var(--tile); border-radius: 8px;
    overflow: hidden; display: flex; align-items: center; justify-content: center;
    min-height: 0;
  }
  .tile video {
    width: 100%%; height: 100%%; object-fit: cover;
  }
  .tile .name-label {
    position: absolute; bottom: 6px; left: 6px;
    background: var(--label-bg); color: var(--text); padding: 2px 8px;
    border-radius: 4px; font-size: .8rem; pointer-events: none;
    max-width: calc(100%% - 12px); overflow: hidden;
    text-overflow: ellipsis; white-space: nowrap;
  }
  .tile .no-video {
    display: flex; align-items: center; justify-content: center;
    width: 100%%; height: 100%%; font-size: 2.5rem;
    background: var(--tile-no-video); color: var(--text-muted);
  }

  /* Self-view PIP */
  #selfView {
    position: fixed; bottom: 80px; right: 12px;
    width: 160px; height: 120px; border-radius: 8px;
    overflow: hidden; background: var(--tile); z-index: 20;
    box-shadow: 0 2px 12px rgba(0,0,0,.5);
  }
  #selfView video {
    width: 100%%; height: 100%%; object-fit: cover;
    transform: scaleX(-1);
  }
  #selfView .no-video {
    display: flex; align-items: center; justify-content: center;
    width: 100%%; height: 100%%; font-size: 1.5rem;
    background: var(--tile-no-video); color: var(--text-muted);
  }

  /* Toolbar */
  #toolbar {
    display: flex; align-items: center; justify-content: center;
    gap: 12px; padding: 12px; background: var(--toolbar);
    z-index: 10; position: relative;
  }
  .tb-btn {
    width: 48px; height: 48px; border-radius: 50%%; border: none;
    background: var(--tb-btn-bg); color: var(--text); font-size: 1.2rem;
    cursor: pointer; display: flex; align-items: center; justify-content: center;
    transition: background .2s;
  }
  .tb-btn:hover { background: var(--tb-btn-hover); }
  .tb-btn.active { background: var(--accent); color: #fff; }
  .tb-btn.hangup { background: var(--accent); color: #fff; }
  .tb-btn.hangup:hover { background: var(--accent-hover); }
  .tb-btn svg { width: 22px; height: 22px; fill: currentColor; }
  .toolbar-watermark {
    position: absolute; right: 16px; font-size: .7rem;
    color: var(--text-muted); opacity: .35; pointer-events: none;
    font-weight: 600; letter-spacing: .5px;
  }

  /* Debug panel — toggle with Ctrl+D */
  #debugPanel {
    display: none; position: fixed; top: 0; right: 0; bottom: 0;
    width: min(420px, 90vw); background: var(--dbg-bg); color: var(--dbg-text);
    font-family: monospace; font-size: .75rem; padding: 12px;
    overflow-y: auto; z-index: 100; border-left: 2px solid var(--border);
  }
  #debugPanel.open { display: block; }
  #debugPanel h3 { color: var(--accent); font-size: .9rem; margin: 8px 0 4px; }
  #debugPanel pre { white-space: pre-wrap; word-break: break-all; margin: 0 0 6px; }
  #debugPanel .dbg-row { padding: 2px 0; border-bottom: 1px solid var(--dbg-border); }
  #debugPanel .dbg-key { color: var(--text-muted); }
  #debugPanel .dbg-val { color: var(--success); }
  #debugPanel .dbg-warn { color: var(--accent); }
  #debugPanel button {
    background: var(--dbg-btn-bg); color: var(--text); border: 1px solid var(--dbg-btn-border);
    padding: 4px 10px; border-radius: 4px; cursor: pointer; margin: 2px;
    font-family: monospace; font-size: .75rem;
  }
  #debugPanel button:hover { background: var(--dbg-btn-hover); }
  #debugPanel input {
    background: var(--dbg-input-bg); color: var(--dbg-text); border: 1px solid var(--border);
    padding: 4px 6px; font-family: monospace; font-size: .75rem;
    width: 100%%; margin: 4px 0;
  }

  /* Connection status */
  #connStatus {
    text-align: center; padding: 8px; font-size: .85rem;
    color: var(--text-muted); background: var(--toolbar); z-index: 5;
  }
  #connStatus.connected { display: none; }

  /* Responsive grid */
  @media (min-width: 600px) {
    #selfView { width: 200px; height: 150px; }
  }
</style>
</head>
<body>

<!-- Join card -->
<div class="card" id="joinCard">
  <div class="theme-picker" id="themePicker">
    <button data-theme="light" title="Light theme">&#9728;</button>
    <button data-theme="dark" title="Dark theme">&#9790;</button>
    <button data-theme="auto" title="Auto (system)">&#9680;</button>
  </div>
  <div class="wally-logo">
    <svg viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg">
      <circle cx="24" cy="24" r="23" stroke="currentColor" stroke-width="2" fill="var(--accent)"/>
      <text x="24" y="32" text-anchor="middle" fill="#fff" font-size="26" font-weight="bold" font-family="-apple-system,BlinkMacSystemFont,sans-serif">W</text>
    </svg>
    <span>Wally Conference</span>
  </div>
  <p class="subtitle">%s</p>
  <p id="breakoutLabel" style="display:none;text-align:center;font-size:.85em;padding:4px 12px;border-radius:6px;background:var(--accent);color:#fff;margin:0 0 8px"></p>
  <form id="joinForm">
    <label for="name">Your name</label>
    <input id="name" type="text" placeholder="Enter your display name" maxlength="50" required autofocus>
    <button type="submit" id="joinBtn" class="join-btn">Join Call</button>
    <div id="msg"></div>
  </form>
  <div class="branding">Powered by <a href="https://github.com/LaPingvino/wally-conference">Wally Conference</a> &mdash; open-source guest calling for Matrix</div>
</div>

<!-- Call view -->
<div id="callView">
  <div id="connStatus">Connecting&hellip;</div>
  <div id="videoGrid"></div>
  <div id="selfView"><div class="no-video">&#128100;</div></div>
  <div id="toolbar">
    <button class="tb-btn" id="btnMic" title="Toggle microphone">
      <svg viewBox="0 0 24 24"><path d="M12 14a3 3 0 003-3V5a3 3 0 10-6 0v6a3 3 0 003 3zm-1 4.93A7.004 7.004 0 015 12h2a5 5 0 0010 0h2a7.004 7.004 0 01-6 6.93V22h-2v-3.07z"/></svg>
    </button>
    <button class="tb-btn" id="btnCam" title="Toggle camera">
      <svg viewBox="0 0 24 24"><path d="M17 10.5V7a1 1 0 00-1-1H4a1 1 0 00-1 1v10a1 1 0 001 1h12a1 1 0 001-1v-3.5l4 4v-11l-4 4z"/></svg>
    </button>
    <button class="tb-btn" id="btnScreen" title="Share screen">
      <svg viewBox="0 0 24 24"><path d="M20 3H4a2 2 0 00-2 2v11a2 2 0 002 2h7v2H8v2h8v-2h-3v-2h7a2 2 0 002-2V5a2 2 0 00-2-2zm0 13H4V5h16v11z"/></svg>
    </button>
    <button class="tb-btn" id="btnBreakout" title="Breakout rooms" style="position:relative">
      <svg viewBox="0 0 24 24"><path d="M3 13h8V3H3v10zm0 8h8v-6H3v6zm10 0h8V11h-8v10zm0-18v6h8V3h-8z"/></svg>
    </button>
    <button class="tb-btn hangup" id="btnHangup" title="Leave call">
      <svg viewBox="0 0 24 24"><path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .39-.23.74-.56.9-.98.49-1.87 1.12-2.66 1.85-.18.18-.43.28-.7.28-.28 0-.53-.11-.71-.29L.29 13.08a.956.956 0 010-1.36C3.46 8.83 7.49 7 12 7s8.54 1.83 11.71 4.72c.18.18.29.44.29.71 0 .28-.11.53-.29.71l-2.48 2.48c-.18.18-.43.29-.71.29-.27 0-.52-.1-.7-.28a11.27 11.27 0 00-2.67-1.85.996.996 0 01-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z"/></svg>
    </button>
    <span class="toolbar-watermark">Wally</span>
  </div>
  <!-- Breakout panel (hidden by default) -->
  <div id="breakoutPanel" style="display:none;position:absolute;bottom:60px;left:50%%;transform:translateX(-50%%);background:var(--card);border:1px solid var(--border);border-radius:8px;padding:12px;min-width:240px;max-width:320px;z-index:10;box-shadow:0 4px 12px rgba(0,0,0,.3)">
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
      <strong style="font-size:.9em">Breakout Rooms</strong>
      <span id="breakoutClose" style="cursor:pointer;font-size:1.2em">&times;</span>
    </div>
    <div id="breakoutList" style="max-height:200px;overflow-y:auto;margin-bottom:8px"><em style="opacity:.6;font-size:.85em">Loading...</em></div>
    <button id="btnReturnMain" class="join-btn" style="width:100%%;font-size:.85em;padding:6px">Return to Main Room</button>
  </div>
</div>

<!-- Debug panel (Ctrl+D to toggle) -->
<div id="debugPanel">
  <h3>Wally Debug <span style="float:right;cursor:pointer" onclick="document.getElementById('debugPanel').classList.remove('open')">&times;</span></h3>
  <div id="dbgInfo"></div>
  <h3>LiveKit Room Alias Tester</h3>
  <div>
    <div class="dbg-row"><span class="dbg-key">Matrix Room ID:</span> <span class="dbg-val" id="dbgRoomId"></span></div>
    <label style="color:#888;font-size:.7rem">Suffix (default: m.call#ROOM, try empty):</label>
    <input id="dbgSuffix" value="m.call#ROOM" placeholder="m.call#ROOM">
    <button id="dbgCalcHash">Compute alias</button>
    <pre id="dbgHashResult"></pre>
  </div>
  <h3>Reconnect with custom alias</h3>
  <input id="dbgCustomAlias" placeholder="Paste a LiveKit room alias to try">
  <button id="dbgReconnect">Reconnect to this room</button>
  <pre id="dbgReconnectResult"></pre>
  <h3>Participants</h3>
  <div id="dbgParticipants"></div>
  <h3>Event Log</h3>
  <div id="dbgLog" style="max-height:200px;overflow-y:auto"></div>
</div>

<script src="https://cdn.jsdelivr.net/npm/livekit-client@2.18.0/dist/livekit-client.umd.js"></script>
<script>
(function() {
  // ── Theme picker ──
  function setTheme(choice) {
    localStorage.setItem('wally-theme', choice);
    var resolved = choice;
    if (choice === 'auto') {
      resolved = window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
    }
    document.documentElement.setAttribute('data-theme', resolved);
    document.querySelectorAll('.theme-picker button').forEach(function(b) {
      b.classList.toggle('active', b.dataset.theme === choice);
    });
  }
  // Init picker state
  var savedTheme = localStorage.getItem('wally-theme') || 'dark';
  document.querySelectorAll('.theme-picker button').forEach(function(b) {
    b.classList.toggle('active', b.dataset.theme === savedTheme);
    b.addEventListener('click', function() { setTheme(b.dataset.theme); });
  });
  // Listen for OS theme changes when in auto mode
  window.matchMedia('(prefers-color-scheme: light)').addEventListener('change', function() {
    if (localStorage.getItem('wally-theme') === 'auto') setTheme('auto');
  });

  const roomId = %q;
  const breakoutId = new URLSearchParams(window.location.search).get('breakout') || '';
  if (breakoutId) {
    var lbl = document.getElementById('breakoutLabel');
    lbl.textContent = 'Breakout Room: ' + breakoutId;
    lbl.style.display = 'block';
  }
  const form = document.getElementById('joinForm');
  const nameInput = document.getElementById('name');
  const btn = document.getElementById('joinBtn');
  const msgEl = document.getElementById('msg');

  const callView = document.getElementById('callView');
  const videoGrid = document.getElementById('videoGrid');
  const selfViewEl = document.getElementById('selfView');

  const btnMic = document.getElementById('btnMic');
  const btnCam = document.getElementById('btnCam');
  const btnScreen = document.getElementById('btnScreen');
  const btnHangup = document.getElementById('btnHangup');

  const LK = window.LivekitClient;
  let room = null;
  let micEnabled = true;
  let camEnabled = true;
  let screenEnabled = false;
  let guestSessionId = null;
  let currentBreakoutId = breakoutId || null;
  const btnBreakout = document.getElementById('btnBreakout');
  const breakoutPanel = document.getElementById('breakoutPanel');

  // ── Helpers ──

  function avatarInitial(name) {
    return (name || '?').charAt(0).toUpperCase();
  }

  function updateGridLayout() {
    const count = videoGrid.children.length;
    let cols = 1;
    if (count >= 2) cols = 2;
    if (count >= 5) cols = 3;
    if (count >= 10) cols = 4;
    videoGrid.style.gridTemplateColumns = 'repeat(' + cols + ', 1fr)';
  }

  function ensureTile(participant) {
    const id = participant.sid || participant.identity;
    let tile = videoGrid.querySelector('[data-participant="' + id + '"]');
    if (!tile) {
      tile = document.createElement('div');
      tile.className = 'tile';
      tile.dataset.participant = id;

      const noVid = document.createElement('div');
      noVid.className = 'no-video';
      noVid.textContent = avatarInitial(participant.name || participant.identity);
      tile.appendChild(noVid);

      const label = document.createElement('div');
      label.className = 'name-label';
      label.textContent = participant.name || participant.identity || 'Guest';
      tile.appendChild(label);

      videoGrid.appendChild(tile);
      updateGridLayout();
    }
    return tile;
  }

  function removeTile(participant) {
    const id = participant.sid || participant.identity;
    const tile = videoGrid.querySelector('[data-participant="' + id + '"]');
    if (tile) {
      // Detach any tracks
      tile.querySelectorAll('video, audio').forEach(function(el) { el.srcObject = null; el.remove(); });
      tile.remove();
      updateGridLayout();
    }
  }

  function attachTrackToTile(track, participant) {
    if (track.kind === 'audio') {
      // Audio: hidden element appended to body
      const el = track.attach();
      el.id = 'audio-' + participant.sid + '-' + track.sid;
      el.style.display = 'none';
      document.body.appendChild(el);
      return;
    }
    if (track.kind === 'video') {
      // Screen share tracks go into their own tile-like element
      const isScreen = track.source === LK.Track.Source.ScreenShare;
      const tile = ensureTile(participant);

      const videoEl = track.attach();
      videoEl.style.width = '100%%';
      videoEl.style.height = '100%%';
      videoEl.style.objectFit = isScreen ? 'contain' : 'cover';
      videoEl.dataset.trackSid = track.sid;

      // Hide avatar placeholder when video is attached
      const noVid = tile.querySelector('.no-video');
      if (noVid) noVid.style.display = 'none';

      // Insert before the label
      const label = tile.querySelector('.name-label');
      tile.insertBefore(videoEl, label);
    }
  }

  function detachTrack(track, participant) {
    if (track.kind === 'audio') {
      const el = document.getElementById('audio-' + participant.sid + '-' + track.sid);
      if (el) { el.srcObject = null; el.remove(); }
      return;
    }
    if (track.kind === 'video') {
      const id = participant.sid || participant.identity;
      const tile = videoGrid.querySelector('[data-participant="' + id + '"]');
      if (tile) {
        const vid = tile.querySelector('video[data-track-sid="' + track.sid + '"]');
        if (vid) { vid.srcObject = null; vid.remove(); }
        // Show avatar placeholder if no more video
        if (!tile.querySelector('video')) {
          const noVid = tile.querySelector('.no-video');
          if (noVid) noVid.style.display = '';
        }
      }
    }
  }

  function updateSelfView() {
    selfViewEl.innerHTML = '';
    if (!room || !room.localParticipant) return;
    const camTrack = room.localParticipant.getTrackPublication(LK.Track.Source.Camera);
    if (camTrack && camTrack.track && !camTrack.isMuted) {
      const el = camTrack.track.attach();
      selfViewEl.appendChild(el);
    } else {
      const noVid = document.createElement('div');
      noVid.className = 'no-video';
      noVid.textContent = avatarInitial(room.localParticipant.name || room.localParticipant.identity);
      selfViewEl.appendChild(noVid);
    }
  }

  // ── Join flow ──

  function showStatus(text, isError) {
    msgEl.className = isError ? 'error' : 'success';
    msgEl.textContent = text;
  }

  form.addEventListener('submit', async function(e) {
    e.preventDefault();
    const displayName = nameInput.value.trim();
    if (!displayName) return;
    btn.disabled = true;
    btn.textContent = 'Requesting media access\u2026';
    msgEl.textContent = '';
    msgEl.className = '';

    // Request media permissions BEFORE connecting — user sees the browser
    // prompt on the join card rather than a blank call view.
    var localStream = null;
    try {
      localStream = await navigator.mediaDevices.getUserMedia({audio: true, video: true});
    } catch (mediaErr) {
      // Camera denied/unavailable — try audio only
      console.warn('Camera not available, trying audio only:', mediaErr);
      try {
        localStream = await navigator.mediaDevices.getUserMedia({audio: true});
        showStatus('Camera unavailable \u2014 joining with audio only', false);
      } catch (audioErr) {
        console.warn('No media devices available:', audioErr);
        showStatus('No camera or microphone found \u2014 joining as listener', false);
      }
    }

    // Stop the preview stream — LiveKit will request its own
    var hasVideo = false, hasAudio = false;
    if (localStream) {
      hasVideo = localStream.getVideoTracks().length > 0;
      hasAudio = localStream.getAudioTracks().length > 0;
      localStream.getTracks().forEach(function(t) { t.stop(); });
    }

    btn.textContent = 'Connecting\u2026';
    try {
      const resp = await fetch('./join', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({room_id: roomId, display_name: displayName, breakout_id: breakoutId || undefined}),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || 'Join failed');
      guestSessionId = data.session_id;
      await startCall(data.livekit_url, data.jwt, displayName, hasAudio, hasVideo);
    } catch (err) {
      showStatus(err.message, true);
      btn.disabled = false;
      btn.textContent = 'Join Call';
    }
  });

  async function startCall(livekitUrl, jwt, displayName, hasAudio, hasVideo) {
    // Hide join card, show call view
    document.getElementById('joinCard').style.display = 'none';
    callView.classList.add('active');
    document.body.style.padding = '0';

    room = new LK.Room({
      adaptiveStream: true,
      dynacast: true,
    });

    // ── Room events ──

    room.on(LK.RoomEvent.TrackSubscribed, function(track, publication, participant) {
      dbgLog('TrackSubscribed: ' + (participant.name||participant.identity) + ' ' + track.source + ':' + track.kind);
      attachTrackToTile(track, participant);
    });

    room.on(LK.RoomEvent.TrackUnsubscribed, function(track, publication, participant) {
      dbgLog('TrackUnsubscribed: ' + (participant.name||participant.identity) + ' ' + track.source + ':' + track.kind);
      detachTrack(track, participant);
    });

    room.on(LK.RoomEvent.ParticipantConnected, function(participant) {
      dbgLog('ParticipantConnected: ' + participant.identity + ' (' + (participant.name||'?') + ')');
      ensureTile(participant);
    });

    room.on(LK.RoomEvent.ParticipantDisconnected, function(participant) {
      dbgLog('ParticipantDisconnected: ' + participant.identity);
      removeTile(participant);
    });

    room.on(LK.RoomEvent.LocalTrackPublished, function(publication, participant) {
      updateSelfView();
    });

    room.on(LK.RoomEvent.LocalTrackUnpublished, function(publication, participant) {
      updateSelfView();
    });

    room.on(LK.RoomEvent.TrackMuted, function(publication, participant) {
      if (participant === room.localParticipant) {
        updateSelfView();
      }
    });

    room.on(LK.RoomEvent.TrackUnmuted, function(publication, participant) {
      if (participant === room.localParticipant) {
        updateSelfView();
      }
    });

    room.on(LK.RoomEvent.Disconnected, function() {
      leaveCall();
    });

    room.on(LK.RoomEvent.ConnectionStateChanged, function(state) {
      dbgLog('ConnectionState: ' + state);
      var el = document.getElementById('connStatus');
      if (state === LK.ConnectionState.Connected) {
        el.className = 'connected';
        dbgLog('Connected to LK room: ' + (room.name || '?') + ' sid=' + (room.sid || '?'));
      } else if (state === LK.ConnectionState.Reconnecting) {
        el.className = '';
        el.textContent = 'Reconnecting\u2026';
      }
    });

    // Connect
    await room.connect(livekitUrl, jwt);

    // Publish media based on what permissions we got
    if (hasVideo && hasAudio) {
      try {
        await room.localParticipant.enableCameraAndMicrophone();
      } catch (err) {
        console.warn('Could not enable camera/mic:', err);
        try { await room.localParticipant.setMicrophoneEnabled(true); } catch (e2) { console.warn('Mic fallback failed:', e2); }
      }
    } else if (hasAudio) {
      try { await room.localParticipant.setMicrophoneEnabled(true); } catch (err) { console.warn('Could not enable mic:', err); }
      micEnabled = true; camEnabled = false;
      btnCam.classList.add('active');
    } else {
      // No media — listener mode
      micEnabled = false; camEnabled = false;
      btnMic.classList.add('active');
      btnCam.classList.add('active');
    }

    updateSelfView();

    // Create tiles for already-connected participants
    room.remoteParticipants.forEach(function(participant) {
      ensureTile(participant);
      participant.trackPublications.forEach(function(pub) {
        if (pub.track && pub.isSubscribed) {
          attachTrackToTile(pub.track, participant);
        }
      });
    });
  }

  // ── Controls ──

  btnMic.addEventListener('click', async function() {
    if (!room) return;
    micEnabled = !micEnabled;
    await room.localParticipant.setMicrophoneEnabled(micEnabled);
    btnMic.classList.toggle('active', !micEnabled);
    btnMic.title = micEnabled ? 'Mute microphone' : 'Unmute microphone';
  });

  btnCam.addEventListener('click', async function() {
    if (!room) return;
    camEnabled = !camEnabled;
    await room.localParticipant.setCameraEnabled(camEnabled);
    btnCam.classList.toggle('active', !camEnabled);
    btnCam.title = camEnabled ? 'Disable camera' : 'Enable camera';
    updateSelfView();
  });

  btnScreen.addEventListener('click', async function() {
    if (!room) return;
    try {
      screenEnabled = !screenEnabled;
      await room.localParticipant.setScreenShareEnabled(screenEnabled);
      btnScreen.classList.toggle('active', screenEnabled);
    } catch (err) {
      console.warn('Screen share error:', err);
      screenEnabled = false;
      btnScreen.classList.remove('active');
    }
  });

  btnHangup.addEventListener('click', function() {
    leaveCall();
  });

  function leaveCall() {
    if (room) {
      room.disconnect();
      room = null;
    }
    // Clean up audio elements
    document.querySelectorAll('audio[id^="audio-"]').forEach(function(el) {
      el.srcObject = null; el.remove();
    });
    // Return to join card
    callView.classList.remove('active');
    videoGrid.innerHTML = '';
    selfViewEl.innerHTML = '<div class="no-video">&#128100;</div>';
    document.getElementById('joinCard').style.display = '';
    document.body.style.padding = '1rem';
    btn.disabled = false;
    btn.textContent = 'Join Call';
    micEnabled = true;
    camEnabled = true;
    screenEnabled = false;
    btnMic.classList.remove('active');
    btnCam.classList.remove('active');
    btnScreen.classList.remove('active');
  }
  // ── Breakout rooms ──

  btnBreakout.addEventListener('click', function() {
    var isOpen = breakoutPanel.style.display !== 'none';
    breakoutPanel.style.display = isOpen ? 'none' : 'block';
    if (!isOpen) loadBreakouts();
  });
  document.getElementById('breakoutClose').addEventListener('click', function() {
    breakoutPanel.style.display = 'none';
  });

  async function loadBreakouts() {
    var list = document.getElementById('breakoutList');
    list.innerHTML = '<em style="opacity:.6;font-size:.85em">Loading...</em>';
    try {
      var resp = await fetch('./breakout/list/' + encodeURIComponent(roomId));
      if (!resp.ok) { list.innerHTML = '<em style="opacity:.6;font-size:.85em">Not available</em>'; return; }
      var data = await resp.json();
      var breakouts = data.breakouts || [];
      if (breakouts.length === 0) {
        list.innerHTML = '<em style="opacity:.6;font-size:.85em">No breakout rooms</em>';
        return;
      }
      list.innerHTML = '';
      breakouts.forEach(function(br) {
        var div = document.createElement('div');
        div.style.cssText = 'display:flex;justify-content:space-between;align-items:center;padding:4px 0;border-bottom:1px solid var(--border)';
        var info = document.createElement('span');
        info.style.cssText = 'font-size:.85em;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1';
        info.textContent = (br.topic || br.id) + ' (' + br.participants + ')';
        var joinBtn = document.createElement('button');
        joinBtn.textContent = currentBreakoutId === br.id ? 'Current' : 'Join';
        joinBtn.disabled = currentBreakoutId === br.id;
        joinBtn.style.cssText = 'margin-left:8px;padding:2px 10px;border:none;border-radius:4px;background:var(--accent);color:#fff;cursor:pointer;font-size:.8em';
        joinBtn.addEventListener('click', function() { moveToBreakout(br.id); });
        div.appendChild(info);
        div.appendChild(joinBtn);
        list.appendChild(div);
      });
    } catch (err) {
      list.innerHTML = '<em style="opacity:.6;font-size:.85em">Error: ' + err.message + '</em>';
    }
  }

  async function moveToBreakout(targetBreakoutId) {
    if (!guestSessionId) { alert('No active session'); return; }
    try {
      var resp = await fetch('./breakout/move', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({session_id: guestSessionId, breakout_id: targetBreakoutId}),
      });
      var data = await resp.json();
      if (!resp.ok) throw new Error(data.error || 'Move failed');
      // Disconnect from current room and reconnect to breakout
      if (room) { room.disconnect(); room = null; }
      currentBreakoutId = targetBreakoutId;
      var wasAudio = micEnabled, wasVideo = camEnabled;
      await startCall(data.livekit_url, data.jwt, displayName, wasAudio, wasVideo);
      breakoutPanel.style.display = 'none';
    } catch (err) {
      alert('Failed to move: ' + err.message);
    }
  }

  document.getElementById('btnReturnMain').addEventListener('click', async function() {
    if (!guestSessionId || !currentBreakoutId) return;
    // Re-join main room by creating a new session
    try {
      var resp = await fetch('./join', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({room_id: roomId, display_name: displayName}),
      });
      var data = await resp.json();
      if (!resp.ok) throw new Error(data.error || 'Rejoin failed');
      guestSessionId = data.session_id;
      currentBreakoutId = null;
      if (room) { room.disconnect(); room = null; }
      var wasAudio = micEnabled, wasVideo = camEnabled;
      await startCall(data.livekit_url, data.jwt, displayName, wasAudio, wasVideo);
      breakoutPanel.style.display = 'none';
    } catch (err) {
      alert('Failed to return: ' + err.message);
    }
  });

  // ── Debug panel ──

  var dbgData = {};  // stored join response

  function dbgLog(msg) {
    var el = document.getElementById('dbgLog');
    var t = new Date().toISOString().slice(11,23);
    el.innerHTML = '<div class="dbg-row">[' + t + '] ' + msg + '</div>' + el.innerHTML;
  }

  function dbgUpdateInfo() {
    var el = document.getElementById('dbgInfo');
    var html = '';
    function row(k, v, cls) {
      html += '<div class="dbg-row"><span class="dbg-key">' + k + ':</span> <span class="' + (cls||'dbg-val') + '">' + (v||'—') + '</span></div>';
    }
    row('Room ID', roomId);
    row('Connection', room ? room.state : 'not connected');
    if (dbgData.livekit_url) row('LK URL', dbgData.livekit_url);
    if (dbgData.livekit_room) row('LK Room (grant)', dbgData.livekit_room);
    if (dbgData.debug) {
      row('Alias input', dbgData.debug.alias_input);
      row('LK identity', dbgData.debug.lk_identity);
      row('Device ID', dbgData.debug.device_id);
      row('State key', dbgData.debug.state_key);
      row('LK service URL', dbgData.debug.lk_service_url);
    }
    if (room && room.name) row('LK Room (actual)', room.name);
    if (room && room.sid) row('LK Room SID', room.sid);
    if (room && room.localParticipant) {
      row('Local identity', room.localParticipant.identity);
      row('Local SID', room.localParticipant.sid);
      var pubs = [];
      room.localParticipant.trackPublications.forEach(function(p) {
        pubs.push(p.source + ':' + p.kind + (p.isMuted ? '(muted)' : ''));
      });
      row('Local tracks', pubs.join(', ') || 'none');
    }
    el.innerHTML = html;
  }

  function dbgUpdateParticipants() {
    var el = document.getElementById('dbgParticipants');
    if (!room) { el.innerHTML = '<div class="dbg-row">Not connected</div>'; return; }
    var html = '';
    html += '<div class="dbg-row"><span class="dbg-key">Local:</span> <span class="dbg-val">' +
      (room.localParticipant.identity || '?') + ' (' + (room.localParticipant.name || '?') + ')</span></div>';
    room.remoteParticipants.forEach(function(p) {
      var tracks = [];
      p.trackPublications.forEach(function(pub) {
        tracks.push(pub.source + ':' + pub.kind + (pub.isSubscribed ? '' : '(unsub)') + (pub.isMuted ? '(muted)' : ''));
      });
      html += '<div class="dbg-row"><span class="dbg-key">' + (p.name || p.identity) + ':</span> <span class="dbg-val">' +
        p.identity + ' [' + (tracks.join(', ') || 'no tracks') + ']</span></div>';
    });
    if (room.remoteParticipants.size === 0) {
      html += '<div class="dbg-row dbg-warn">No remote participants in this LK room</div>';
    }
    el.innerHTML = html;
  }

  // SHA-256 + unpadded base64 in browser
  async function computeAlias(input) {
    var enc = new TextEncoder();
    var hash = await crypto.subtle.digest('SHA-256', enc.encode(input));
    var bytes = new Uint8Array(hash);
    var binary = '';
    for (var i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
    // Standard base64, strip padding
    return btoa(binary).replace(/=+$/, '');
  }

  document.getElementById('dbgRoomId').textContent = roomId;

  document.getElementById('dbgCalcHash').addEventListener('click', async function() {
    var suffix = document.getElementById('dbgSuffix').value;
    var input = roomId + '|' + suffix;
    var alias = await computeAlias(input);
    var result = 'Input: ' + input + '\nAlias: ' + alias;
    if (dbgData.livekit_room) {
      result += '\nJWT room: ' + dbgData.livekit_room;
      result += '\nMatch: ' + (alias === dbgData.livekit_room ? 'YES' : 'NO — MISMATCH!');
    }
    document.getElementById('dbgHashResult').textContent = result;
  });

  document.getElementById('dbgReconnect').addEventListener('click', async function() {
    var customAlias = document.getElementById('dbgCustomAlias').value.trim();
    if (!customAlias || !dbgData.livekit_url) {
      document.getElementById('dbgReconnectResult').textContent = 'Need alias and a previous join';
      return;
    }
    dbgLog('Reconnecting to custom room: ' + customAlias);
    // We need a new JWT for the custom room — re-join with override
    try {
      var resp = await fetch('./join', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({room_id: roomId, display_name: room ? room.localParticipant.name : 'Debug'}),
      });
      var data = await resp.json();
      if (!resp.ok) throw new Error(data.error || 'Join failed');
      // Note: JWT is locked to the room alias in the grant, so we can only
      // test with the alias the server computed. Log the mismatch.
      document.getElementById('dbgReconnectResult').textContent =
        'JWT room: ' + data.livekit_room + '\nCustom: ' + customAlias +
        '\n⚠ JWT grants are room-locked — to test a different alias, the server must compute it.';
    } catch (err) {
      document.getElementById('dbgReconnectResult').textContent = 'Error: ' + err.message;
    }
  });

  // Toggle with Ctrl+D
  document.addEventListener('keydown', function(e) {
    if (e.ctrlKey && e.key === 'd') {
      e.preventDefault();
      document.getElementById('debugPanel').classList.toggle('open');
      dbgUpdateInfo();
      dbgUpdateParticipants();
    }
  });

  // Hook into join flow to capture debug data
  var _origFetch = window.fetch;
  window.fetch = async function() {
    var resp = await _origFetch.apply(this, arguments);
    if (arguments[0] === './join') {
      var clone = resp.clone();
      clone.json().then(function(data) {
        dbgData = data;
        dbgLog('Join response: room=' + data.livekit_room + ' url=' + data.livekit_url);
        dbgUpdateInfo();
      }).catch(function(){});
    }
    return resp;
  };

  // Periodic debug refresh while connected
  setInterval(function() {
    if (room && document.getElementById('debugPanel').classList.contains('open')) {
      dbgUpdateInfo();
      dbgUpdateParticipants();
    }
  }, 2000);

  // Hook room events for debug log
  var _origStartCall = startCall;
  // We can't easily wrap startCall, so hook into room events after connect.
  // The RoomEvent hooks above already exist; add debug logging via MutationObserver on videoGrid.
  var gridObserver = new MutationObserver(function() {
    dbgUpdateParticipants();
  });
  gridObserver.observe(videoGrid, {childList: true, subtree: true});
})();
</script>
</body>
</html>`

// HandleGuestPage serves a self-contained HTML guest join page.
func (svc *Service) HandleGuestPage(w http.ResponseWriter, r *http.Request) {
	rawRoomID := r.PathValue("roomID")
	if rawRoomID == "" {
		http.Error(w, "Missing room ID", http.StatusBadRequest)
		return
	}
	roomID, err := url.PathUnescape(rawRoomID)
	if err != nil {
		roomID = rawRoomID
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, guestPageTemplate, html.EscapeString(roomID), roomID)
}
