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
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #1a1a2e; color: #eee;
    display: flex; align-items: center; justify-content: center;
    min-height: 100vh; padding: 1rem;
    overflow: hidden;
  }

  /* ── Join card ── */
  .card {
    background: #16213e; border-radius: 12px; padding: 2rem;
    max-width: 400px; width: 100%%; box-shadow: 0 4px 24px rgba(0,0,0,.4);
  }
  h1 { font-size: 1.4rem; margin-bottom: .25rem; }
  .subtitle { color: #888; font-size: .85rem; margin-bottom: 1.5rem; word-break: break-all; }
  label { display: block; font-size: .9rem; margin-bottom: .4rem; color: #aaa; }
  input {
    width: 100%%; padding: .65rem .8rem; border-radius: 8px;
    border: 1px solid #333; background: #0f3460; color: #eee;
    font-size: 1rem; margin-bottom: 1rem; outline: none;
  }
  input:focus { border-color: #e94560; }
  .join-btn {
    width: 100%%; padding: .7rem; border-radius: 8px; border: none;
    background: #e94560; color: #fff; font-size: 1rem; font-weight: 600;
    cursor: pointer; transition: background .2s;
  }
  .join-btn:hover { background: #c73e54; }
  .join-btn:disabled { background: #555; cursor: not-allowed; }
  .error { color: #e94560; font-size: .85rem; margin-top: .5rem; }
  .success { color: #4ec94e; font-size: .85rem; margin-top: .5rem; }
  .branding { text-align: center; margin-top: 1.5rem; color: #555; font-size: .75rem; }
  .branding a { color: #888; text-decoration: none; }

  /* ── Call view ── */
  #callView {
    display: none; position: fixed; inset: 0;
    background: #1a1a2e; flex-direction: column;
  }
  #callView.active { display: flex; }

  #videoGrid {
    flex: 1; display: grid; gap: 4px; padding: 4px;
    grid-template-columns: 1fr;
    overflow: hidden;
  }

  .tile {
    position: relative; background: #16213e; border-radius: 8px;
    overflow: hidden; display: flex; align-items: center; justify-content: center;
    min-height: 0;
  }
  .tile video {
    width: 100%%; height: 100%%; object-fit: cover;
  }
  .tile .name-label {
    position: absolute; bottom: 6px; left: 6px;
    background: rgba(0,0,0,.6); color: #eee; padding: 2px 8px;
    border-radius: 4px; font-size: .8rem; pointer-events: none;
    max-width: calc(100%% - 12px); overflow: hidden;
    text-overflow: ellipsis; white-space: nowrap;
  }
  .tile .no-video {
    display: flex; align-items: center; justify-content: center;
    width: 100%%; height: 100%%; font-size: 2.5rem;
    background: #0f3460; color: #aaa;
  }

  /* Self-view PIP */
  #selfView {
    position: fixed; bottom: 80px; right: 12px;
    width: 160px; height: 120px; border-radius: 8px;
    overflow: hidden; background: #16213e; z-index: 20;
    box-shadow: 0 2px 12px rgba(0,0,0,.5);
  }
  #selfView video {
    width: 100%%; height: 100%%; object-fit: cover;
    transform: scaleX(-1);
  }
  #selfView .no-video {
    display: flex; align-items: center; justify-content: center;
    width: 100%%; height: 100%%; font-size: 1.5rem;
    background: #0f3460; color: #aaa;
  }

  /* Toolbar */
  #toolbar {
    display: flex; align-items: center; justify-content: center;
    gap: 12px; padding: 12px; background: #16213e;
    z-index: 10;
  }
  .tb-btn {
    width: 48px; height: 48px; border-radius: 50%%; border: none;
    background: #2a2a4a; color: #eee; font-size: 1.2rem;
    cursor: pointer; display: flex; align-items: center; justify-content: center;
    transition: background .2s;
  }
  .tb-btn:hover { background: #3a3a5a; }
  .tb-btn.active { background: #e94560; }
  .tb-btn.hangup { background: #e94560; }
  .tb-btn.hangup:hover { background: #c73e54; }
  .tb-btn svg { width: 22px; height: 22px; fill: currentColor; }

  /* Connection status */
  #connStatus {
    text-align: center; padding: 8px; font-size: .85rem;
    color: #888; background: #16213e; z-index: 5;
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
  <h1>Join Call</h1>
  <p class="subtitle">%s</p>
  <form id="joinForm">
    <label for="name">Your name</label>
    <input id="name" type="text" placeholder="Enter your display name" maxlength="50" required autofocus>
    <button type="submit" id="joinBtn" class="join-btn">Join Call</button>
    <div id="msg"></div>
  </form>
  <div class="branding">Powered by <a href="https://github.com/LaPingvino/wally-conference">Wally Conference</a></div>
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
    <button class="tb-btn hangup" id="btnHangup" title="Leave call">
      <svg viewBox="0 0 24 24"><path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .39-.23.74-.56.9-.98.49-1.87 1.12-2.66 1.85-.18.18-.43.28-.7.28-.28 0-.53-.11-.71-.29L.29 13.08a.956.956 0 010-1.36C3.46 8.83 7.49 7 12 7s8.54 1.83 11.71 4.72c.18.18.29.44.29.71 0 .28-.11.53-.29.71l-2.48 2.48c-.18.18-.43.29-.71.29-.27 0-.52-.1-.7-.28a11.27 11.27 0 00-2.67-1.85.996.996 0 01-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z"/></svg>
    </button>
  </div>
</div>

<script src="https://cdn.jsdelivr.net/npm/livekit-client@2.18.0/dist/livekit-client.umd.js"></script>
<script>
(function() {
  const roomId = %q;
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
        body: JSON.stringify({room_id: roomId, display_name: displayName}),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || 'Join failed');
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
      attachTrackToTile(track, participant);
    });

    room.on(LK.RoomEvent.TrackUnsubscribed, function(track, publication, participant) {
      detachTrack(track, participant);
    });

    room.on(LK.RoomEvent.ParticipantConnected, function(participant) {
      ensureTile(participant);
    });

    room.on(LK.RoomEvent.ParticipantDisconnected, function(participant) {
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
      var el = document.getElementById('connStatus');
      if (state === LK.ConnectionState.Connected) {
        el.className = 'connected';
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
