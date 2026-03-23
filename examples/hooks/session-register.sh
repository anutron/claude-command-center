#!/bin/bash
# examples/hooks/session-register.sh
#
# Claude Code hook for automatic CCC session registration.
# Add to your Claude Code settings.json:
#
#   "hooks": {
#     "session_start": [{
#       "command": "/usr/local/bin/ccc register --session-id $SESSION_ID --pid $PPID --project $PWD"
#     }]
#   }
#
# For named sessions, add to your CLAUDE.md:
#
#   After understanding the session topic, register it with CCC:
#   ```bash
#   ccc update-session --session-id $SESSION_ID --topic "Your Topic Here"
#   ```

ccc register --session-id "$SESSION_ID" --pid "$PPID" --project "$PWD"
