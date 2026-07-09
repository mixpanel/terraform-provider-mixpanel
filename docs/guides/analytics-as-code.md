---
page_title: "Analytics as code"
subcategory: "Getting Started"
description: |-
  Manage your Mixpanel analytics estate — cohorts, metrics, taxonomy, and governance — as version-controlled code that is reviewed in pull requests and applied by Terraform.
---

# Analytics as code

Your Mixpanel project is one of the most consequential systems your company
runs on, and for most teams it is also among the least governed. The cohorts,
metrics, dashboards, and event taxonomy that everyone trusts are built by
clicking through a web app, and that is where they live — as state inside a UI,
with no history and no review.

This guide makes the case for managing that estate the way engineering teams
already manage their infrastructure: as version-controlled code. It is written
for the Mixpanel product manager who owns the metrics, and second for the
engineer who will wire it up.

---

## The problem: analytics as unversioned click-state

When your analytics configuration lives as clicks in a web app and nowhere
else, three problems compound over time.

- **No history.** A cohort definition changes and the numbers on a dashboard
  move with it. Who changed it? When? Why? The UI cannot answer. There is no
  diff to read, no author to ask, no reason recorded. The audit trail is
  whatever people happen to remember.

- **Dev and prod drift apart.** Most teams keep separate Mixpanel projects for
  development, staging, and production. Each was configured by hand, so each is
  subtly different. A metric that works in staging behaves differently in prod
  because a filter was set up slightly differently months ago, and nobody can
  see the gap until it produces a wrong number.

- **Taxonomy decay.** Event and property names accrete duplicates — `Sign Up`,
  `signup`, `sign_up` — because there is no reviewed gate on what gets added.
  The Lexicon (Mixpanel's dictionary of your events and properties) slowly
  fills with near-synonyms, and every report built on top inherits the mess.

None of this is a discipline failure. It is the predictable result of keeping
important configuration in a place that has no version control.

---

## The move: define it as code

Infrastructure as code means describing the state you want in text files,
committing those files to git, and letting a tool make the reality match. For
Mixpanel, the tool is **Terraform** — an open-source program that reads your
configuration and makes the API calls needed to reach the state it describes.

You write the configuration in HCL (Terraform's configuration language). Each
`resource` block describes one Mixpanel object you want to exist. Here is a
compact, real example: a cohort, a saved metric, and a Lexicon tag, all in one
file.

```terraform
# A cohort — the audience you care about, defined once and versioned.
resource "mixpanel_cohort" "power_users" {
  name       = "Power Users"
  project_id = 1234567
  groups = jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = "$all_users"
        label        = "All Users"
      }
      filters                   = []
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
}

# A metric — a saved measurement your team reports on.
resource "mixpanel_metric" "signups" {
  name       = "Signups"
  type       = "metric"
  project_id = 1234567
  definition = jsonencode({
    display = {}
    behavior = {
      name         = "Sign Up"
      type         = "event"
      search       = ""
      dataset      = "$mixpanel"
      filters      = []
      resourceType = "events"
    }
    measurement = {
      math       = "unique"
      cumulative = false
    }
  })
}

# A Lexicon tag — taxonomy governance, in the same repo as everything else.
resource "mixpanel_lexicon_tag" "acquisition" {
  project_id  = 1234567
  name        = "acquisition"
  description = "User acquisition and onboarding events"
  color       = "#4CAF50"
}
```

Once the configuration exists, the working loop is three steps in plain words:

1. **Plan.** `terraform plan` is a dry run. It compares your files against what
   is really in Mixpanel and prints exactly what it would create, change, or
   delete — and it changes nothing. You read the plan the way you would read a
   receipt before paying.

2. **Review.** Because the configuration is text in git, a teammate reviews the
   change in a pull request (a PR — the change-review unit of git) before it
   goes anywhere. The plan output can be attached to the PR so reviewers see
   both the intended edit and its computed effect.

3. **Apply.** `terraform apply` executes the approved plan against the Mixpanel
   API. The reality now matches the file, and the file is the record of why.

---

## What you get

Moving durable analytics config into code buys concrete, checkable things.

- **Review and blame for analytics.** Every change is a diff a teammate
  approves before it reaches production. Afterward, `git blame` on the cohort
  file shows who last touched each line and links to the change that did it —
  the "who, when, and why" the UI could never give you.

- **Environment parity.** The same files deploy to your dev, staging, and
  production projects, so they stop diverging: each is generated from one
  source instead of configured by hand. See
  [Cross-project portability](./cross-project-portability.md).

- **Drift detection.** If someone edits a managed cohort directly in the
  Mixpanel UI, the provider notices. That out-of-band edit is called *drift*,
  and the next `terraform plan` shows it as a difference; the next apply either
  converges it back to the file or you fold the change into the file on
  purpose. Either way it is visible. See
  [Drift detection](./drift-detection.md).

- **Scale without copy-paste.** `for_each` is a Terraform loop: write a cohort
  or a tag once and stamp it across ten projects or twenty events, keeping the
  logic in a single place.

- **Governance as code.** Your Lexicon taxonomy — tags, event and property
  definitions, drop filters — lives in the same reviewed repo as the metrics
  that depend on it. Adding an event name becomes a reviewable diff instead of
  silent accretion, which is how taxonomy decay stops.

- **Bootstrap and recovery.** A new project's analytics estate is one
  `terraform apply` away. If a project is misconfigured or lost, the
  configuration is the backup you rebuild from.

---

## The agentic multiplier

There is a second reason this matters now. AI coding assistants are far more
capable at reading and writing code than at clicking through a web app, so the
leverage of an AI agent is highest wherever work is expressed as code — and
analytics as code puts your Mixpanel estate squarely in that category.

The mechanics matter more than the enthusiasm. `terraform plan` plus PR review
is a review harness that fits agent-proposed change exactly: an agent edits the
configuration, `plan` computes the precise set of objects that would change, and
a human approves the PR before anything is applied. Nothing reaches Mixpanel
that a person did not see rendered in a plan.

One concrete scenario. A PM asks an agent to "set up analytics for the Q3
launch." The agent opens a PR that adds a launch cohort, three launch metrics,
and a `launch-q3` Lexicon tag. `terraform plan` on that PR renders the exact
objects that would be created — names, filters, tag colors — as a diff. The PM
reads the diff, asks for one metric to switch from total to unique counting,
approves, and applies. If the agent had gotten something wrong, it would have
been a line in a diff caught in review, not a silent change discovered later in
production.

---

## What stays in the UI

Not everything belongs in code, and pretending otherwise would waste your time.

Ad-hoc exploration stays in the Mixpanel UI, where it is fast and interactive:
slicing a funnel, testing a hypothesis, chasing down a one-off question. That
work is exploratory by nature and does not benefit from a review gate.

Durable, shared artifacts belong in code: the cohorts, metrics, dashboards, and
taxonomy your team depends on week over week. The dividing line is whether the
object is something people rely on repeatedly, not who built it or where it
started.

The two modes coexist by design. Build something in the UI, and when it proves
worth keeping, `terraform import` brings it under management as configuration;
drift detection then keeps the file and the live object honest with each other.
See [Importing existing Mixpanel objects](./import.md).

---

## Where this stands

As of mid-2026, managing product analytics with Terraform is still uncommon,
and the available options are narrow.

- **PostHog** is the one product-analytics vendor with a comparable official
  Terraform provider, covering roughly 18 resources.
- **Amplitude** and **Heap** ship no vendor-official Terraform provider of
  their own.

Against that backdrop, this provider stands out on breadth and depth together.
It spans 43 resources and 46 data sources across the analytical, governance,
and administrative surfaces of Mixpanel — not just event configuration, but
cohorts, metrics, formulas, dashboards, Lexicon governance, experiments, data
pipelines, and org and access administration. The point is not a count for its
own sake; it is that a large fraction of what you can do in the Mixpanel UI you
can also express, review, and version as code.

For the provider's current maturity and testing posture, and a map of every
supported capability, see the [provider index](../index.md).

---

## Where to go next

- [Getting started](./getting-started.md) — install Terraform or OpenTofu,
  write your initial configuration, and run the plan/apply loop end to end.
- [Importing existing Mixpanel objects](./import.md) — bring objects you
  already built in the UI under Terraform management.
- [Cross-project portability](./cross-project-portability.md) — deploy one
  configuration across dev, staging, and production projects.
- [Drift detection](./drift-detection.md) — how the provider detects and
  reconciles changes made outside Terraform.
