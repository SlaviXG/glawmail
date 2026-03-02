---
name: glawmail
description: Format emails in GLAWMAIL format for human-in-the-loop sending via Gmail. Use when asked to send email, compose email, or email someone.
metadata:
  version: 1.0
  created: 2026-03-01
  repo: https://github.com/SlaviXG/glawmail
---

# GlawMail Skill

Format emails for human-in-the-loop Gmail sending.

## Format

```
GLAWMAIL
To: recipient@email.com
Subject: Email subject line
Body:
Your email body here...
```

## Rules

- Start with `GLAWMAIL` on its own line
- `To:` followed by email address
- `Subject:` followed by subject line
- `Body:` followed by the message (can be multiline)
- User forwards your message to GlawMail bot to send

## Triggers

Use this format when user says:
- "send email"
- "email to"
- "compose email"
- "write email"

## Example

User: "Send email to james@example.com about the meeting tomorrow"

Response:
```
GLAWMAIL
To: james@example.com
Subject: Meeting Tomorrow
Body:
Hi James,

Just a reminder about our meeting tomorrow.

Best regards
```

## Security

- You cannot send emails directly
- User must forward to GlawMail bot to approve
- Human-in-the-loop design prevents unauthorized sending
