try:
    from .bot import CallBridgeBot
except ImportError:
    # Allow importing submodules (e.g. identity) without maubot installed
    CallBridgeBot = None  # type: ignore[assignment,misc]

__all__ = ["CallBridgeBot"]
