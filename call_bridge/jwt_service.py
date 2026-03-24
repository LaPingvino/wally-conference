"""LiveKit JWT issuance using the livekit-api package."""

from datetime import timedelta

from livekit import api


def make_guest_jwt(
    lk_api_key: str,
    lk_api_secret: str,
    livekit_room_alias: str,
    participant_identity: str,
    participant_name: str,
    ttl_seconds: int = 7200,
) -> str:
    """Generate a LiveKit access token (JWT) for a guest participant.

    Args:
        lk_api_key: LiveKit API key (becomes the JWT ``iss`` claim).
        lk_api_secret: LiveKit API secret (HS256 signing key).
        livekit_room_alias: SHA256-derived LiveKit room name.
        participant_identity: SHA256-derived participant identity.
        participant_name: Human-readable display name for the participant.
        ttl_seconds: Token lifetime in seconds (default 2 hours).

    Returns:
        A signed JWT string.
    """
    token = (
        api.AccessToken(api_key=lk_api_key, api_secret=lk_api_secret)
        .with_identity(participant_identity)
        .with_name(participant_name)
        .with_ttl(timedelta(seconds=ttl_seconds))
        .with_grants(
            api.VideoGrants(
                room=livekit_room_alias,
                room_join=True,
                can_publish=True,
                can_subscribe=True,
                can_publish_data=True,
                can_update_own_metadata=True,
            )
        )
    )
    return token.to_jwt()
