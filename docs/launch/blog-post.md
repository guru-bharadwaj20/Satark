---
title: "Satark: thirty seconds before you click"
description: "Why I built an open-source scam checker for Indian messages, links and QR codes, and how it works."
tags: [india, security, kubernetes, opensource, golang]
---

# Satark: thirty seconds before you click

My aunt got a message last year: her bank account would be blocked by evening unless she updated her KYC at a link. The link looked almost right. She didn't click it, but only because she called me first, and I happened to pick up.

Most people don't have someone to call. Messages like that work because they look official, they create panic, and in the moment there's no quick way to check. India has good channels for *after* something goes wrong: the **1930** helpline, **cybercrime.gov.in**, and **Chakshu** on Sanchar Saathi. I wanted something for the thirty seconds *before*.

So I built **Satark** (सतर्क, "alert"). It's free and open source.

## What it does

Forward a suspicious message to the Satark Telegram bot, share it from your phone's share menu to the web app, or send a screenshot of a QR code. In a few seconds you get:

- a risk level (high, be careful, or no warning signs found),
- the reasons, in plain English, Hindi or Kannada (a lookalike domain registered last week, an "account will be blocked" threat, a UPI *collect* request disguised as a refund),
- what to do next.

If you've been targeted, the **Report** button saves the report and gives you the exact steps for 1930, cybercrime.gov.in and Chakshu, with the details ready to paste into the form.

You can also look up a phone number or UPI ID and see whether other people have reported it.

## What it deliberately doesn't do

A tool like this can do harm if it's careless, so a few rules are built in:

- **It never says "100% safe".** A clean result says *no warning signs found, which doesn't guarantee it's safe*. Scams change faster than any checker.
- **It never calls a person a scammer.** Community data shows how many *independent* people reported something, and only after at least three have. Numbers are masked (`98XXXXXX21`), counts are rounded, and anyone wrongly reported can ask for a review.
- **It doesn't keep what you check.** Phone numbers, UPI IDs, account numbers and OTPs are stripped out *before* any text goes to an AI model. `/forget` deletes everything Satark has about you.
- **It never types into a suspicious site.** Links open in a throwaway, isolated browser that records redirects and takes a screenshot, and is destroyed afterwards.

## How it works

Satark is a set of small services on Kubernetes:

- A **gateway** fans each check out to a **text analyzer** (rules plus an LLM for explanations, on redacted text only), a **URL intelligence** service (redirects, domain age, TLS, lookalikes, threat feeds), a **QR decoder** that understands UPI deep links, and a **reputation** service for community reports.
- Risky links go to a **Kubernetes operator** that starts a one-off sandboxed browser pod under gVisor with egress-only networking.
- Reports are stored only as keyed hashes, so the database can't be turned back into a list of people's numbers.

Under that is the part I enjoyed most: the cluster is built with Terraform and Ansible, deployed by Argo CD from Git, policed by Kyverno, monitored with Prometheus and Grafana, and has runbooks, game days and an AI on-call assistant that reads them. The [architecture document](../architecture.md) has the diagrams.

## Try it

- Telegram: **@SatarkBot** _(link at launch)_
- Web app: **satark.example** _(install it on Android to get it in your share menu)_
- Code: **github.com/guru-bharadwaj20/satark**

If you're a developer building something for a residents' association, a college or an NGO, there's a free [API](../api/README.md).

## A last word

Satark gives a risk assessment, not a guarantee. It will sometimes be wrong. Banks and government bodies never ask for your OTP, UPI PIN or password. If you've lost money, **call 1930 immediately**; the first hours matter.

And forward this to the person in your family who always picks up the phone.
