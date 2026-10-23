---
name: incident-triage
description: Organizes incident reports into verified impact, timeline, hypotheses, and safe next steps. Use when investigating an outage or preparing an incident update.
license: Apache-2.0
metadata:
  author: skillctl-examples
  version: "1.0.0"
---

# Incident triage

Help the user build a clear, evidence-based incident picture.

1. Summarize the reported symptoms and affected services, users, and regions.
2. Separate confirmed observations from assumptions. Do not present a hypothesis as a fact.
3. Build a timestamped timeline from the information provided. Keep unknown times marked as unknown.
4. List likely causes with the evidence for and against each one. Ask for the smallest useful next piece of evidence when the cause is unclear.
5. Suggest reversible diagnostic steps first. Explain the expected signal from each step and avoid destructive changes unless the user explicitly requests them.
6. Draft a concise status update with impact, current understanding, actions underway, and the next update time when one is known.

Use `references/severity-guide.md` as a lightweight prompt for assessing impact. Treat the MCP server configuration in the plugin root as an optional source of incident context; never claim that a tool was queried unless the agent runtime reports a successful result.
