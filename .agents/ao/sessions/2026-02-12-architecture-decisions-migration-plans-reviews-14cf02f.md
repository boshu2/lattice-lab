---
session_id: 14cf02fd-2530-4d2f-83cc-0eaa3b08242d
date: 2026-02-12
summary: "architecture decisions, migration plans
- Reviews where multiple valid perspectives exist
- Cases..."
tags:
  - olympus
  - session
  - 2026-02
---

# architecture decisions, migration plans
- Reviews where multiple valid perspectives exist
- Cases...

**Session:** 14cf02fd-2530-4d2f-83cc-0eaa3b08242d
**Date:** 2026-02-12

## Decisions
- architecture decisions, migration plans
- Reviews where multiple valid perspectives exist
- Cases where a missed finding has real consequences

Skip `--debate` for routine validation where consensus...

## Knowledge
- Key Insight:** Skills and hooks communicate through a narrow corridor of flag files and additionalContext strings; widening this to a structured skill-lifecycle bus would unlock automatic context...
- key insight is that the current architecture is a set of point-to-point integrations (hooks.json manually wires specific hooks to specific events), and the highest-leverage change is inverting this...
- The fix is inverting the dependency so **skills declare their own hooks**.

### Top 5 Ideas (by leverage)

| # | Idea | Impact | Effort |
|---|------|--------|--------|
| 1 | **Skill-Declared Hook...
- insight: skills and hooks communicate through narrow ad-hoc channels. The fix is inverting the dependency so **skills declare their own hooks**.

### Top 5 Ideas (by leverage)

| # | Idea | Impact |...
- Key Takeaway

**AgentOps is broader** (more hooks, multi-agent, ecosystem thinking) but **Olympus's Meta-Loop insight is sharper**: the idea that the agent should be a pure function with hooks...
- till FAIL after 3 total attempts → stop with message:
        "Pre-mortem failed 3 times. Last report: <path>. Manual intervention needed."

3. Store verdict in `rpi_state.verdicts.pre_mortem`

4....
- til all issues assigned

#### Validate Dependency Necessity

For EACH declared dependency, verify:
1. Does the blocked issue modify a file that the blocker also modifies? → **Keep**
2. Does the...
- til 3+ skills need gating.

3. **Auto-handoff (ag-9ad.3)** is the highest-value item but is under-scoped -- the hard part (modifying the 307-line `session-start.sh` to inject handoff data) is not...
- til the epic is DONE.

**CLI dependencies:** bd (issue tracking), ao (knowledge flywheel). Both optional — see `skills/shared/SKILL.md` for fallback table. If bd is unavailable, use TaskList for...

## Files Changed
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/hooks.json]]
- [[/Users/fullerbt/gt/agentops/crew/nami]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/ratchet-advance.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/pre-mortem-gate.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/prompt-nudge.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/task-validation-gate.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/standards-injector.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/skills/SKILL-TIERS.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/session-start.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/push-gate.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/stop-team-guard.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/council/2026-02-12-brainstorm-skills-hooks-judge-1.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/council/2026-02-12-brainstorm-skills-hooks-judge-2.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/council/2026-02-12-brainstorm-skills-hooks-integration.md]]
- [[/Users/fullerbt/gt/olympus/crew/goku/.agents/council/2026-02-12-brainstorm-ol-hooks-report.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/rpi/phase-1-summary.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/hooks/precompact-snapshot.sh]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/rpi/phase-2-summary.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/plans]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/plans/2026-02-12-skills-hooks-integration.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/council/2026-02-12-pre-mortem-judge-spec-completeness.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/council/2026-02-12-pre-mortem-judge-scope.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/council/2026-02-12-pre-mortem-skills-hooks.md]]
- [[/Users/fullerbt/gt/agentops/crew/nami/.agents/rpi/phase-3-summary.md]]

## Issues
- [[issues/sub-agents|sub-agents]]
- [[issues/and-forget|and-forget]]
- [[issues/pre-commit|pre-commit]]
- [[issues/na-0042|na-0042]]
- [[issues/re-spawn|re-spawn]]
- [[issues/pre-mortem|pre-mortem]]
- [[issues/low-effort|low-effort]]
- [[issues/to-point|to-point]]
- [[issues/ad-hoc|ad-hoc]]
- [[issues/bug-hunt|bug-hunt]]
- [[issues/ol-hooks-report|ol-hooks-report]]
- [[issues/pre-mortem-gate|pre-mortem-gate]]
- [[issues/to-phase|to-phase]]
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
- [[issues/ag-9ad|ag-9ad]]
- [[issues/two-round|two-round]]
- [[issues/pre-mortem-judge|pre-mortem-judge]]
- [[issues/rev-parse|rev-parse]]
- [[issues/per-task|per-task]]
- [[issues/pre-next-wave|pre-next-wave]]

## Tool Usage

| Tool | Count |
|------|-------|
| Bash | 35 |
| Edit | 4 |
| Glob | 1 |
| Read | 22 |
| Skill | 3 |
| Task | 8 |
| Write | 3 |

## Tokens

- **Input:** 0
- **Output:** 0
- **Total:** ~259773 (estimated)
