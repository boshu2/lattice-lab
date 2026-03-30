---
session_id: e0a00a79-f4e4-42ba-a525-b5ef9b463e8e
date: 2026-02-12
summary: "decision:** Hybrid approach — default `ao hooks install` stays minimal (flywheel hooks only, ba..."
tags:
  - olympus
  - session
  - 2026-02
---

# decision:** Hybrid approach — default `ao hooks install` stays minimal (flywheel hooks only, ba...

**Session:** e0a00a79-f4e4-42ba-a525-b5ef9b463e8e
**Date:** 2026-02-12

## Decisions
- decision:** Hybrid approach — default `ao hooks install` stays minimal (flywheel hooks only, backwards compatible). New `ao hooks install --full` copies all 12 scripts to `~/.agentops/hooks/` and...
- architecture decisions, migration plans
- Reviews where multiple valid perspectives exist
- Cases where a missed finding has real consequences

Skip `--debate` for routine validation where consensus...
- decision: hybrid approach (default minimal, --full for complete, --agent for cross-agent).
   
   - **All 12 hook scripts** were read during research: session-start.sh, pending-cleaner.sh,...

## Knowledge
- till FAIL after 3 total attempts → stop with message:
        "Pre-mortem failed 3 times. Last report: <path>. Manual intervention needed."

3. Store verdict in `rpi_state.verdicts.pre_mortem`

4....
- key insight is that `ao hooks install` should read from the canonical `hooks/hooks.json` rather than hardcoding hooks in Go, and it should support generating configs for different agents.
- til all issues assigned

#### Validate Dependency Necessity

For EACH declared dependency, verify:
1. Does the blocked issue modify a file that the blocker also modifies? → **Keep**
2. Does the...
- Fixed by using PATH="/usr/bin:/bin" to hide ao for fallback tests.

Summary:
1. Primary Request and Intent:
   The user's session progressed through several related requests:
   - Initially asked how...
- til Wave 3 tests
4. Stale "Ask First" items already decided in the same doc — could confuse workers

4 significant, 3 minor findings. All addressable with conformance table additions before...
- till inverted from the original `bd dep add` direction issue, but the plan document has the correct execution order. Crank will read the plan for wave ordering.

Now let me write the phase summary...
- til the epic is DONE.

**CLI dependencies:** bd (issue tracking), ao (knowledge flywheel). Both optional — see `skills/shared/SKILL.md` for fallback table. If bd is unavailable, use TaskList for...

## Files Changed
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/hooks.json]]
- [[/Users/fullerbt/gt/agentops/crew/nami]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.claude-plugin/plugin.json]]
- [[/Users/fullerbt/gt/agentops/crew/nami/tests/hooks/test-hooks.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/tests/run-all.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/session-start.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/lib/skills-core.js]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/task-validation-gate.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/lib/hook-helpers.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/standards-injector.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/git-worker-guard.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/dangerous-git-guard.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/ratchet-advance.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/stop-team-guard.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/pre-mortem-gate.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/research/2026-02-12-hook-installation.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/rpi/phase-1-summary.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/cli/cmd/ao/hooks.go]]
- [[/Users/fullerbt/gt/agentops/crew/nami/schemas/hooks-manifest.v1.schema.json]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/plans/2026-02-12-hook-installation.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/council/judge-missing-requirements.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/council/judge-feasibility.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/council/judge-scope.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/council/judge-spec-completeness.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/council/2026-02-12-pre-mortem-hook-installation.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/rpi/phase-3-summary.md]]

## Issues
- [[issues/git-worker-guard|git-worker-guard]]
- [[issues/git-guard|git-guard]]
- [[issues/pre-mortem-gate|pre-mortem-gate]]
- [[issues/pre-mortem|pre-mortem]]
- [[issues/non-worker|non-worker]]
- [[issues/non-crank|non-crank]]
- [[issues/in-process|in-process]]
- [[issues/ag-23k|ag-23k]]
- [[issues/max-cycles|max-cycles]]
- [[issues/re-plan|re-plan]]
- [[issues/re-crank|re-crank]]
- [[issues/re-invoke|re-invoke]]
- [[issues/new-goal|new-goal]]
- [[issues/dry-run|dry-run]]
- [[issues/sub-skill|sub-skill]]
- [[issues/re-run|re-run]]
- [[issues/ol-571|ol-571]]
- [[issues/sub-issues|sub-issues]]
- [[issues/of-scope|of-scope]]
- [[issues/na-0001|na-0001]]
- [[issues/on-wave-1|on-wave-1]]
- [[issues/na-0002|na-0002]]
- [[issues/per-issue|per-issue]]
- [[issues/in-session|in-session]]
- [[issues/ag-6jt|ag-6jt]]
- [[issues/two-round|two-round]]
- [[issues/sub-agents|sub-agents]]
- [[issues/and-forget|and-forget]]
- [[issues/pre-commit|pre-commit]]
- [[issues/na-0042|na-0042]]
- [[issues/re-spawn|re-spawn]]
- [[issues/non-plugin|non-plugin]]
- [[issues/in-repo|in-repo]]
- [[issues/per-event|per-event]]
- [[issues/pre-mortem-hook|pre-mortem-hook]]
- [[issues/rev-parse|rev-parse]]
- [[issues/per-task|per-task]]
- [[issues/pre-next-wave|pre-next-wave]]
- [[issues/pre-flight|pre-flight]]

## Tool Usage

| Tool | Count |
|------|-------|
| Bash | 61 |
| Edit | 13 |
| Glob | 6 |
| Grep | 3 |
| Read | 30 |
| SendMessage | 4 |
| Skill | 4 |
| Task | 8 |
| TaskCreate | 4 |
| TeamCreate | 1 |
| TeamDelete | 1 |
| Write | 8 |

## Tokens

- **Input:** 0
- **Output:** 0
- **Total:** ~654054 (estimated)
