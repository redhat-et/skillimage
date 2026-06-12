---
name: tweetclaw-openclaw
description: Package an approval-gated OpenClaw plugin workflow for TweetClaw and Xquik.
license: Apache-2.0
metadata:
  author: skillimage-contributors
  version: "1.0.0"
---

Use this skill when an OpenClaw agent needs X/Twitter data or reviewed
account actions through TweetClaw.

## Install

Install the published OpenClaw plugin before using this skill:

```bash
openclaw plugins install npm:@xquik/tweetclaw
```

Configure the Xquik API key in OpenClaw config or a local environment file.
Do not paste private values into chats, prompts, runbooks, logs, or generated
artifacts.

## Tools

Use `explore` first to inspect available TweetClaw operations. Use `tweetclaw`
only after the user has confirmed the intended account, task, and side effects.

## Good Tasks

- scrape tweets and search tweets
- search tweet replies
- export followers and look up users
- upload or download media
- monitor tweets and deliver webhooks
- run giveaway draws
- post tweets or post replies after explicit review

## Approval Boundary

Treat posting, replying, direct messages, follows, profile changes, webhook
creation, new monitors, and giveaway draws as approval-gated actions. Present
the structured action request first and wait for user approval before calling
`tweetclaw`.

Keep drafting, scoring, scheduling, analytics, and voice decisions in the
calling workflow. TweetClaw supplies X/Twitter evidence and approved execution,
but the agent remains responsible for its own review policy.
