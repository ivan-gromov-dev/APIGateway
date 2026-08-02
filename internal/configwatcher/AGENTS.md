# Config watcher instructions

The watcher is a platform-neutral polling scheduler. It owns no gateway
state: callers provide the reload callback. Never log or expose configuration
contents, and always stop the ticker when the context is cancelled.
