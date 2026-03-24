# Element Call Patch

Call Bridge requires a small patch to Element Call that allows it to accept a pre-issued LiveKit JWT via URL parameter, bypassing the normal OpenID token exchange.

## What the patch does

Normally, Element Call:
1. Gets an OpenID token from the Matrix homeserver
2. Sends it to lk-jwt-service
3. Receives a LiveKit JWT
4. Connects to LiveKit

The patch adds a shortcut: if `livekitToken` is present as a URL parameter, EC skips steps 1-3 and uses the provided token directly. This is how guests (who have no Matrix account) connect to LiveKit.

## The patch is safe

- **Inert by default**: Only activates when `livekitToken` URL param is present
- **No behavioral change** for normal Matrix users who connect via OpenID
- **No security downgrade**: The JWT is still validated by LiveKit (signature check)
- **Reversible**: Remove the patch and guests simply can't join (graceful)

## Patch location

In Element Call source: `src/livekit/openIDSFU.ts`

At the beginning of the JWT acquisition function, before the OpenID exchange:

```typescript
// Guest mode: accept pre-issued JWT from Wally Conference bot
const urlParams = new URLSearchParams(window.location.search);
const preIssuedJwt = urlParams.get("livekitToken");
const preIssuedRoom = urlParams.get("livekitRoom");

if (preIssuedJwt && preIssuedRoom) {
  const sfuUrl = urlParams.get("livekitUrl") ?? fallbackSfuUrl;
  return { url: sfuUrl, jwt: preIssuedJwt };
}
```

## Guest EC URL format

The bot's `/join` endpoint returns an `ec_url` that loads EC with all necessary parameters:

```
https://cinny.yourserver.com/public/element-call/index.html
  ?embed=true
  &widgetId=guest-widget
  &roomId=!abc:yourserver.com
  &livekitToken=eyJhbGciOiJIUzI1NiIs...
  &livekitRoom=base64sha256hash
  &livekitUrl=wss://livekit.yourserver.com
  &displayName=Alice+Guest
  &skipLobby=true
  &header=none
  &perParticipantE2EE=false
```

## For Wally/Cinny fork users

The patch is included in `02-element-call.patch` in the Wally patch stack. If you build from the [Codeberg repo](https://codeberg.org/lapingvino/cinny), it's already applied.

## For other Element Call deployments

Apply the patch manually to your EC build. The exact file and line numbers depend on your EC version. The key function to modify is wherever EC acquires the LiveKit JWT (look for `openIdToken` or `getOpenIdToken` calls).

## For non-patched clients

If you can't patch EC (e.g., using stock Element Web), guests joining via Call Bridge will:
- Be audible/visible at the LiveKit level (audio/video works)
- Appear in the Matrix room state (via bot's call.member event)
- Show up in Element Web's call participant list (since call.member is there)

The patch is only strictly needed for the *guest's own browser* to load EC without Matrix auth. Other participants' clients don't need the patch to see the guest.
