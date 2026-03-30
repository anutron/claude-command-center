# internal/agent/

## Summary

- Behavioral branches: 114
- Covered: 97
- Uncovered (behavioral): 7
- Uncovered (implementation detail): 10
- Contradictions: 0
- Unimplemented spec promises: 0

Note: Some items (e.g., 105) represent multiple delegating methods counted as a single branch.

## Branch Coverage

### NewRunner (impl.go)

1. **[COVERED]** maxConcurrent <= 0 defaults to 10 -- Spec: SS Runner -- "NewRunner(maxConcurrent) creates a concrete defaultRunner (defaults to 10 if maxConcurrent <= 0)."
2. **[COVERED]** maxConcurrent > 0 uses provided value -- Spec: SS Runner -- same section, implied by the default clause.

### LaunchOrQueue / defaultRunner (impl.go)

3. **[COVERED]** Request ID already active: returns (false, nil) -- Spec: SS Runner Launching -- "Dedup check: if the request ID is already active or queued, returns (false, nil) -- silently ignored."
4. **[COVERED]** Request ID already queued: returns (false, nil) -- Spec: SS Runner Launching -- same dedup text.
5. **[COVERED]** Under concurrency limit: launches immediately -- Spec: SS Runner Launching -- "If under the concurrency limit, launches immediately via launchSession."
6. **[COVERED]** At concurrency limit: appends to queue, returns (true, nil) -- Spec: SS Runner Launching -- "Otherwise, appends to a FIFO queue and returns (true, nil)."

### launchSession (impl.go)

7. **[COVERED]** Generates UUID for session ID upfront -- Spec: SS Runner Launching step 4 -- "Generates a UUID for the Claude session ID upfront."
8. **[COVERED]** Builds args with --verbose -- Spec: SS Runner Launching step 4 -- "Builds CLI args: claude --verbose [--session-id UUID | --resume ID] [--permission-mode MODE] [--worktree]."
9. **[COVERED]** ResumeID set: uses --resume instead of --session-id -- Spec: SS Runner Launching step 4 -- same CLI args text.
10. **[COVERED]** Permission != "" and != "default": passes --permission-mode -- Spec: SS Runner Launching step 4 -- same CLI args text.
11. **[COVERED]** Worktree true: passes --worktree -- Spec: SS Runner Launching step 4 -- same CLI args text.
12. **[COVERED]** PTY start succeeds: process launched, session registered -- Spec: SS Runner Launching step 4 -- "Starts the process via PTY (pty.Start)."
13. **[COVERED]** PTY start fails: emits SessionFinishedMsg with exit code -1 -- Spec: SS Error cases -- "PTY start fails: emits SessionFinishedMsg with exit code -1, logs error."
14. **[COVERED]** PTY stdout drained to /dev/null -- Spec: SS Runner Launching step 4 -- "Drains PTY stdout to /dev/null (events come from the native log, not stdout)."
15. **[COVERED]** Prompt written as plain text to PTY -- Spec: SS Runner Launching step 4 -- "Writes the prompt as plain text to the PTY."
16. **[COVERED]** Returns SessionStartedMsg -- Spec: SS Runner Launching step 4 -- "Returns SessionStartedMsg."
17. **[UNCOVERED-IMPLEMENTATION]** ProjectDir empty: falls back to os.Getwd() for native log path computation -- Internal detail; the spec says "working directory for the agent process" but the fallback to cwd is an implementation convenience.
18. **[UNCOVERED-IMPLEMENTATION]** Prompt empty: no write to PTY -- Defensive guard; no distinct behavioral outcome worth specifying.

### monitorSessionFromLog (impl.go)

19. **[COVERED]** Opens CCC session log file -- Spec: SS Session Logging -- "Each session gets its own JSONL log file."
20. **[COVERED]** Starts tailNativeLog goroutine -- Spec: SS Monitoring step 1 -- "tailNativeLog polls the Claude native JSONL log file."
21. **[COVERED]** Serializes events to CCC log and output buffer -- Spec: SS Monitoring step 5 -- "Serializes each event to CCC's own session log file and the session output buffer."
22. **[COVERED]** Parses events into SessionEvent, pushes to EventsCh -- Spec: SS Monitoring step 5 -- "Parses events into SessionEvent structs and pushes them to EventsCh (buffered channel, capacity 64)."
23. **[COVERED]** Detects blocking events (SendUserMessage/AskUser) -- Spec: SS Monitoring step 5 -- "Detects blocking events: tool_use with name SendUserMessage or AskUser sets session status to blocked."
24. **[COVERED]** Extracts token usage, invokes CostCallback -- Spec: SS Monitoring step 5 -- "Extracts token usage from assistant events with stop_reason and invokes the CostCallback."
25. **[COVERED]** Per-session budget enforcement: SIGINT when cumulative cost > budget -- Spec: SS Monitoring step 5 -- "Per-session budget enforcement: if cumulative cost exceeds Budget, sends SIGINT to the process."
26. **[COVERED]** Process exits: drains remaining events for up to 2 seconds -- Spec: SS Monitoring step 6 -- "When the process exits, drains remaining log events for up to 2 seconds."
27. **[COVERED]** Records exit code, closes done channel -- Spec: SS Monitoring step 6 -- "then records the exit code and closes the done channel."
28. **[COVERED]** EventsCh full: events dropped silently -- Spec: SS Edge cases -- "Event channel full (64 capacity): events dropped silently (non-blocking send)."
29. **[UNCOVERED-IMPLEMENTATION]** CostCallback nil check before invocation -- Defensive nil guard; no behavioral distinction.
30. **[UNCOVERED-IMPLEMENTATION]** budgetUSD <= 0 skips per-session enforcement -- Implied by "if >= $0.50" in the Request description but not explicitly stated as a skip condition. Implementation detail since zero budget means no enforcement.

### Kill (impl.go)

31. **[COVERED]** Session found: removes from map, closes PTY, kills process, returns true -- Spec: SS Killing -- "Kill(id) removes the session from the active map, closes the PTY (sends SIGHUP to the process group), then calls Process.Kill()."
32. **[COVERED]** Session not found: returns false -- Spec: SS Runner interface -- "Returns true if a session was found and killed." (implies false otherwise)

### SendMessage (impl.go)

33. **[COVERED]** Session found: writes to PTY, resets blocked to processing -- Spec: SS Other -- "SendMessage(id, message) writes to the PTY stdin and resets status from blocked to processing."
34. **[COVERED]** Session not found: returns error -- Spec: SS Runner interface -- implied by "error" return type.
35. **[UNCOVERED-BEHAVIORAL]** PTY is nil: returns error "session PTY is not available" -- The spec describes SendMessage as writing to PTY stdin but does not address the case where the PTY handle is nil after session creation. Intent question: Should the spec note that SendMessage fails if the PTY is unavailable (e.g., after Kill)?

### Status (impl.go)

36. **[COVERED]** Active session: returns status with SessionID, Question, StartedAt -- Spec: SS Runner interface -- "Status returns the current state of a session."
37. **[COVERED]** Queued session: returns status "queued" -- Implied by the interface; the queue is checked.
38. **[COVERED]** Unknown session: returns nil -- Spec: SS Runner interface -- "or nil if unknown."

### Active (impl.go)

39. **[COVERED]** Returns info for all active sessions -- Spec: SS Runner interface -- "Active returns info about all currently running sessions."

### QueueLen (impl.go)

40. **[COVERED]** Returns queue length -- Spec: SS Runner interface -- "QueueLen returns the number of sessions waiting in the queue."

### Session (impl.go)

41. **[COVERED]** Returns session or nil -- Spec: SS Runner interface -- "Session returns the raw session handle for a running agent, or nil."

### DrainQueue (impl.go)

42. **[COVERED]** Queue non-empty and capacity available: pops next, returns (req, true) -- Spec: SS Queue draining -- "DrainQueue() pops the next queued request if there is capacity."
43. **[COVERED]** Queue empty or no capacity: returns (zero, false) -- Spec: SS Queue draining -- "Returns the request and true, or zero value and false if nothing to drain."

### CheckProcesses (impl.go)

44. **[COVERED]** Session done: emits SessionFinishedMsg with exit code -- Spec: SS Other -- "CheckProcesses() polls active sessions for completion (via the done channel)."
45. **[COVERED]** Session done with SessionID: emits SessionIDCapturedMsg before SessionFinishedMsg -- Spec: SS Edge cases -- "Process exits before CheckProcesses runs: SessionIDCapturedMsg emitted before SessionFinishedMsg to ensure session ID is persisted."
46. **[COVERED]** Session running with SessionID: emits SessionIDCapturedMsg -- Spec: SS Other -- "Returns batched tea messages for finished, blocked, and session-ID-captured events."
47. **[COVERED]** Session blocked: emits SessionBlockedMsg -- Spec: SS Other -- same text.
48. **[UNCOVERED-IMPLEMENTATION]** No sessions or no events: returns nil -- Internal optimization; no behavioral consequence.

### Watch (impl.go)

49. **[COVERED]** Session found: returns ListenForSessionEvent cmd -- Spec: SS Other -- "Watch(id) returns a tea.Cmd that listens on the session's EventsCh."
50. **[COVERED]** Session not found: returns nil -- Spec: SS Runner interface -- "Returns nil if the session is not found or has no log path."

### Shutdown (impl.go)

51. **[COVERED]** Closes PTYs (SIGHUP), sends SIGINT, waits up to 3s -- Spec: SS Shutdown -- "Shutdown() closes all PTYs (SIGHUP), sends SIGINT to all processes, then waits up to 3 seconds per session for exit."

### CleanupFinished / defaultRunner (impl.go)

52. **[COVERED]** Removes session from active map, closes PTY, returns session -- Spec: SS Cleanup -- "CleanupFinished(id) removes a finished session from the active map, closes its PTY, and returns the session for summary extraction."
53. **[COVERED]** Session not found: returns nil -- Implied by the interface.

### ListenForSessionEvent (impl.go)

54. **[COVERED]** Channel open: returns SessionEventMsg -- Spec: SS Outputs -- "SessionEventMsg{ID, Event} -- parsed event from agent stdout."
55. **[COVERED]** Channel closed: returns SessionEventsDoneMsg -- Spec: SS Outputs -- "SessionEventsDoneMsg{ID} -- event channel closed."

### ParseSessionEvent (impl.go)

56. **[COVERED]** assistant type: parses text and tool_use blocks -- Spec: SS Session Event Parsing -- "assistant_text -- text content from assistant messages" and "tool_use -- tool name, input (truncated to 80 chars), tool ID."
57. **[COVERED]** tool_result type: extracts result text, tool ID, error flag -- Spec: SS Session Event Parsing -- "tool_result -- result text, tool ID correlation, error flag."
58. **[COVERED]** result type: extracts text -- Spec: SS Session Event Parsing -- implied by "assistant_text" for result events.
59. **[COVERED]** error type: extracts error message -- Spec: SS Session Event Parsing -- "error -- error message."
60. **[COVERED]** user type: extracts user message text -- Spec: SS Session Event Parsing -- "user -- user message text."
61. **[COVERED]** system type: extracts system message -- Spec: SS Session Event Parsing -- "system -- system messages (subtypes, session ID)."
62. **[UNCOVERED-IMPLEMENTATION]** Unknown event type: returns nil -- Defensive fallback; not a behavioral distinction.

### ExtractSessionSummary (impl.go)

63. **[COVERED]** Returns last assistant text or result text -- Spec: SS Test Cases Happy path -- "Summary extraction pulls last assistant text or result text from session output."
64. **[UNCOVERED-BEHAVIORAL]** Empty output with exit code 0: returns "Session completed successfully." -- The spec says summary extraction pulls text, but does not describe the fallback messages when output is empty. Intent question: Should the spec describe the default summary strings for empty-output sessions?
65. **[UNCOVERED-BEHAVIORAL]** Empty output with non-zero exit code: returns "Session exited with code N." -- Same gap as above.
66. **[UNCOVERED-IMPLEMENTATION]** Summary truncated to 1000 chars with newline-aware truncation -- Presentation detail, not behavioral contract.

### DetectBlockingEvent (impl.go)

67. **[COVERED]** tool_use with name SendUserMessage or AskUser: returns true -- Spec: SS Monitoring step 5 -- "Detects blocking events: tool_use with name SendUserMessage or AskUser."
68. **[UNCOVERED-BEHAVIORAL]** assistant type with nested tool_use blocks containing SendUserMessage/AskUser: also returns true -- The spec only mentions "tool_use" events, but the code also checks nested tool_use blocks within "assistant" events. Intent question: Should the spec clarify that blocking detection also applies to tool_use blocks nested inside assistant message content?

### NativeLogPath (logtail.go)

69. **[COVERED]** Computes path as ~/.claude/projects/<encoded-path>/<session-id>.jsonl -- Spec: SS Dependencies -- "Claude native log files (~/.claude/projects/<encoded-path>/<session-id>.jsonl)."

### tailNativeLog (logtail.go)

70. **[COVERED]** Waits up to 30s for file to appear, polling every 200ms -- Spec: SS Monitoring step 2 -- "Waits up to 30 seconds for the file to appear (polling every 200ms)."
71. **[COVERED]** File does not appear: exits -- Spec: SS Edge cases -- "Native log file does not appear within 30 seconds: monitoring goroutine exits, session runs blind."
72. **[COVERED]** Reads JSONL lines, sends parsed events -- Spec: SS Monitoring step 3 -- "Once open, reads JSONL lines and sends parsed map[string]interface{} events to a channel."
73. **[COVERED]** No new lines: polls every 200ms -- Spec: SS Monitoring step 4 -- "When no new lines are available, polls every 200ms."

### extractUsageFromEvent (logtail.go)

74. **[COVERED]** Event with message.stop_reason and message.usage: extracts tokens -- Spec: SS Cost Estimation -- "Token usage is extracted from native log events that have message.stop_reason and message.usage fields."

### estimateCost (logtail.go)

75. **[COVERED]** Model contains "opus": uses $15/$75 pricing -- Spec: SS Cost Estimation -- "Opus (model contains opus): $15/M input, $75/M output."
76. **[COVERED]** Model contains "sonnet": uses $3/$15 pricing -- Spec: SS Cost Estimation -- "Sonnet (default): $3/M input, $15/M output."
77. **[UNCOVERED-BEHAVIORAL]** Model is neither opus nor sonnet (or missing): defaults to sonnet pricing -- The spec says "Sonnet (default)" which implies this, but does not explicitly state the fallback behavior for unknown models. Intent question: Should the spec explicitly list the fallback for unknown/missing model names?

### NewBudgetTracker (budget.go)

78. **[COVERED]** Loads emergency stop state from DB on construction -- Spec: SS BudgetTracker -- "Emergency stop survives daemon restarts (loaded from DB on construction)."

### CanLaunch / BudgetTracker (budget.go)

79. **[COVERED]** Emergency stop active: deny -- Spec: SS BudgetTracker CanLaunch -- "Emergency stop active? Deny."
80. **[COVERED]** hourlySpent + budget > HourlyBudget: deny -- Spec: SS BudgetTracker CanLaunch -- "hourlySpent + budget > HourlyBudget? Deny."
81. **[COVERED]** dailySpent + budget > DailyBudget: deny -- Spec: SS BudgetTracker CanLaunch -- "dailySpent + budget > DailyBudget? Deny."
82. **[COVERED]** All checks pass: allow -- Spec: SS BudgetTracker CanLaunch -- "Otherwise, allow."
83. **[UNCOVERED-BEHAVIORAL]** HourlyBudget <= 0: hourly check skipped entirely -- The spec says "hourlySpent + budget > HourlyBudget? Deny" but does not mention that a zero/unset hourly budget disables the check. Intent question: Should the spec note that hourly/daily budget checks are skipped when the limit is zero or unset?

### RecordLaunch (budget.go)

84. **[COVERED]** Inserts cost row, returns ID -- Spec: SS GovernedRunner -- "inserts a cost row via BudgetTracker.RecordLaunch."
85. **[UNCOVERED-IMPLEMENTATION]** DB insert fails: returns 0 -- Error handling detail; the governed runner proceeds regardless.

### RecordCost (budget.go)

86. **[COVERED]** Updates cost row and refreshes cached totals -- Spec: SS BudgetTracker RecordCost -- "Updates the cc_agent_costs row with running cost/token counts. Refreshes cached hourly/daily totals from DB."

### RecordFinished (budget.go)

87. **[COVERED]** Exit code 0: status "completed" -- Spec: SS BudgetTracker RecordFinished -- "Sets ... status (completed or failed)."
88. **[COVERED]** Exit code != 0: status "failed" -- Spec: SS BudgetTracker RecordFinished -- same.
89. **[COVERED]** Refreshes cached totals -- Spec: SS BudgetTracker RecordFinished -- "Refreshes cached totals."

### EmergencyStop (budget.go)

90. **[COVERED]** Sets stopped=true, persists to DB -- Spec: SS EmergencyStop/Resume -- "Toggle the stopped flag in memory and persist to cc_budget_state."

### Resume (budget.go)

91. **[COVERED]** Sets stopped=false, persists to DB -- Spec: SS EmergencyStop/Resume -- same.

### Status / BudgetTracker (budget.go)

92. **[COVERED]** ratio >= 0.95: "critical" -- Spec: SS Warning levels -- "critical -- hourly spend >= 95% of limit."
93. **[COVERED]** ratio >= BudgetWarningPct: "warning" -- Spec: SS Warning levels -- "warning -- hourly spend >= BudgetWarningPct."
94. **[COVERED]** Below thresholds: "none" -- Spec: SS Warning levels -- "none -- below thresholds."

### NewGovernedRunner (governed_runner.go)

95. **[COVERED]** Creates GovernedRunner wrapping inner runner with budget and rate limiter -- Spec: SS GovernedRunner -- "GovernedRunner wraps a Runner and adds pre-launch checks."

### LaunchOrQueue / GovernedRunner (governed_runner.go)

96. **[COVERED]** Budget denied: returns LaunchDeniedMsg -- Spec: SS GovernedRunner LaunchOrQueue -- "Budget check -- calls BudgetTracker.CanLaunch(budget). If denied, returns LaunchDeniedMsg."
97. **[COVERED]** Rate limit denied: returns LaunchDeniedMsg -- Spec: SS GovernedRunner LaunchOrQueue -- "Rate limit check -- calls RateLimiter.CanLaunch(id, automation). If denied, returns LaunchDeniedMsg."
98. **[COVERED]** Records launch cost, wires CostCallback -- Spec: SS GovernedRunner LaunchOrQueue -- "Record launch -- inserts a cost row via BudgetTracker.RecordLaunch, wires a CostCallback."
99. **[COVERED]** Delegates to inner runner -- Spec: SS GovernedRunner LaunchOrQueue -- "Delegate -- calls inner.LaunchOrQueue(req)."
100. **[COVERED]** Inner runner queues: cleans up cost row -- Spec: SS GovernedRunner LaunchOrQueue -- "If the inner runner queued it (concurrency limit), cleans up the cost row."

### CleanupFinished / GovernedRunner (governed_runner.go)

101. **[COVERED]** Delegates to inner, records cost with duration and exit code -- Spec: SS GovernedRunner CleanupFinished -- "Delegates to the inner runner to get the finished session. Looks up the cost row ID, records duration and exit code via RecordFinished."
102. **[UNCOVERED-IMPLEMENTATION]** Cost row not found (ok==false): skips recording -- Defensive check; not a distinct behavioral outcome.
103. **[UNCOVERED-IMPLEMENTATION]** Session nil from inner: skips recording -- Same defensive pattern.

### Shutdown / GovernedRunner (governed_runner.go)

104. **[COVERED]** Delegates to inner runner -- Spec: SS GovernedRunner -- "All other methods delegate directly to the inner runner." (Shutdown comment also confirms no emergency stop on shutdown.)

### Kill, SendMessage, Status, Active, QueueLen, Session, CheckProcesses, DrainQueue, Watch / GovernedRunner

105. **[COVERED]** All delegate to inner runner -- Spec: SS GovernedRunner -- "All other methods delegate directly to the inner runner."

### NewRateLimiter (ratelimit.go)

106. **[COVERED]** Creates stateless rate limiter backed by DB -- Spec: SS RateLimiter -- "Fully stateless in memory; all state comes from DB queries."

### CanLaunch / RateLimiter (ratelimit.go)

107. **[COVERED]** Per-automation hourly cap exceeded: denied -- Spec: SS RateLimiter -- "Per-automation hourly cap -- counts launches for this automation in the last hour. Default cap: 20."
108. **[COVERED]** Automation empty: hourly cap skipped -- Spec: SS RateLimiter -- "Skipped if automation is empty."
109. **[COVERED]** Per-agent-ID cooldown not elapsed: denied -- Spec: SS RateLimiter -- "Per-agent-ID cooldown -- checks time since last launch of this agentID. Default cooldown: 15 minutes."
110. **[COVERED]** Failure backoff active: denied with exponential backoff -- Spec: SS RateLimiter -- "Failure backoff -- counts failures ... applies exponential backoff: min(baseSec * 2^(failures-1), maxSec)."
111. **[COVERED]** Automation empty: failure backoff skipped -- Spec: SS RateLimiter -- "Skipped if automation is empty."
112. **[COVERED]** All checks pass: allowed -- Spec: SS RateLimiter -- implied by the check sequence.
113. **[COVERED]** Default values: cap=20, cooldown=15min, base=60s, max=3600s -- Spec: SS Configuration -- table lists all defaults.

### DB query errors in RateLimiter

114. **[UNCOVERED-BEHAVIORAL]** DB query error in any rate limit check: returns (false, error message) -- The code denies the launch on DB errors (fail-closed). The spec does not describe this behavior. Intent question: Should the spec note that rate limit checks fail closed on DB errors, denying the launch rather than allowing it?

## Unimplemented Spec Promises

None found. Every behavior described in the spec has a corresponding implementation.
