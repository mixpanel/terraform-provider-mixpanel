---
page_title: "Getting started"
subcategory: "Getting Started"
description: |-
  Install Terraform or OpenTofu and go from zero to your first applied Mixpanel change — an annotation, then a real cohort — in one sitting.
---

# Getting started

This guide takes you from nothing installed to your first Mixpanel object
created from code, and it assumes you have never used Terraform before. Every
term is explained the first time it appears. By the end you will have created a
Mixpanel annotation and a real cohort, changed them, watched the tool show you
exactly what it would do before doing it, and cleaned everything up.

If you want the *why* behind managing analytics this way before you start, read
[Analytics as code](./analytics-as-code.md) first. If you just want to get
hands-on, keep reading.

---

## Prerequisites

You need three things before the first command.

- **A Mixpanel service account.** A *service account* is a non-human login that
  programs use to call the Mixpanel API on your behalf, with its own name and
  secret. Create one in Mixpanel under Organization Settings → Service Accounts.
  Give it a role with **access to the project you will manage** — it can only
  create and change things the account itself is allowed to touch, so a
  read-only role will fail on the first write. You can read more in Mixpanel's
  [service accounts documentation](https://developer.mixpanel.com/reference/service-accounts).

- **Your Mixpanel project ID.** This is the numeric ID of the project you want
  to manage (a number like `1234567`). You can find it in Mixpanel under Project
  Settings.

- **Terraform or OpenTofu.** This is the program that reads your files and makes
  the Mixpanel API calls for you. You can use either one — they are
  command-compatible for everything in this guide, so pick whichever your team
  prefers:
  - **Terraform** — [install instructions](https://developer.hashicorp.com/terraform/install).
  - **OpenTofu** — [install instructions](https://opentofu.org/docs/intro/install/)
    (an open-source, drop-in alternative; substitute `tofu` wherever this guide
    says `terraform`).

Confirm the install worked by running `terraform version` (or `tofu version`);
it should print a version number.

### Set your credentials as environment variables

The provider reads your service account name, its secret, and your project ID
from three *environment variables* — named values your shell hands to any
program it launches. Setting them this way keeps secrets out of your
configuration files, which matters because those files get committed to git.

```sh
export MIXPANEL_SERVICE_ACCOUNT="my-service-account"
export MIXPANEL_SERVICE_ACCOUNT_SECRET="your-service-account-secret"
export MIXPANEL_PROJECT_ID="1234567"
```

**Never commit credentials.** Keep the secret in your shell, a `.env` file that
git ignores, or a secrets manager — never hard-coded in a `.tf` file. (If your
project lives in Mixpanel's EU or India region, you will also need to point the
provider at the regional host; see the region note in the
[provider index](../index.md). Otherwise the default is correct.)

---

## Terraform in five concepts

Before the first command, here is the whole mental model. Terraform has only a
handful of moving parts, and once these five click, everything else is detail.

**Configuration** is the set of text files, ending in `.tf`, where you describe
what you want to exist. You do not write step-by-step instructions ("call this
API, then that one"); you describe the desired end state — "there should be a
cohort named Power Users with these filters" — and let the tool figure out the
calls. The language is called HCL (HashiCorp Configuration Language), and it is
deliberately readable: each `resource` block is one Mixpanel object you want,
like one annotation or one cohort.

**State** is Terraform's private notebook, a file it keeps recording every
object it has created for you and the real Mixpanel ID of each one. This is how
the tool knows that the `mixpanel_annotation` in your file is the *same*
annotation as number `12345` living in Mixpanel — so next time it updates that
one instead of making a duplicate. You do not edit the state file by hand; the
tool maintains it. (For a solo first run it is a local file called
`terraform.tfstate`; teams keep it in shared *remote state* so everyone works
from the same notebook.)

**Plan** is a dry run. When you run `terraform plan`, the tool reads your
configuration, reads what actually exists in Mixpanel right now, compares the
two, and prints exactly what it *would* create, change, or delete to make
reality match your files — while changing absolutely nothing. It is a preview
you read like a receipt before you pay: if a plan says it will delete a
dashboard you did not expect, you stop and look before anything happens.

**Apply** is the moment the changes actually happen. `terraform apply` shows you
the same plan one more time, waits for you to type `yes`, and then makes the
Mixpanel API calls to bring reality in line with your configuration. After a
successful apply, the live Mixpanel project matches your files, and your files
are the record of why each object exists.

**Drift** is what happens when the real world moves out from under your files —
someone opens the Mixpanel web app and renames a cohort you manage, or edits its
filters by hand. That out-of-band edit is called *drift*. The next
`terraform plan` re-reads the object from Mixpanel, notices it no longer matches
your configuration, and shows you the gap; the next `terraform apply` converges
Mixpanel back to what your files say (or you fold the manual change into your
files on purpose). Either way, the difference is never silent. See
[Drift detection](./drift-detection.md) for the exact rules.

---

## Step 1: Write your first configuration

Make a new, empty directory and create one file in it called `main.tf`. Paste in
the following. We will start with an **annotation** — a dated note that appears
on your Mixpanel charts (for example, "v2.0 shipped here"). It is the safest and
most visible object to learn on: it changes no data, and it shows up on the
project timeline for everyone right away.

```terraform
terraform {
  required_providers {
    mixpanel = {
      source = "mixpanel/mixpanel"
    }
  }
}

provider "mixpanel" {}

resource "mixpanel_annotation" "launch" {
  date        = "2026-07-08 00:00:00"
  description = "Summer launch"
}
```

Here is what each part is:

- The `terraform { required_providers { ... } }` block tells Terraform which
  *provider* to download. A *provider* is a plugin that teaches Terraform how to
  talk to one specific system — here, `mixpanel/mixpanel`, the Mixpanel
  provider. Terraform fetches it from the public registry for you.
- The `provider "mixpanel" {}` block configures that plugin. It is empty because
  the provider reads your credentials and project ID from the environment
  variables you set earlier, so there is nothing to put here.
- The `resource "mixpanel_annotation" "launch" { ... }` block is the object you
  want to exist. `mixpanel_annotation` is the *type* of object; `"launch"` is a
  local name you choose so you can refer to this one later (it is not sent to
  Mixpanel). The two attributes are `date` (when the annotation sits on the
  timeline, in `YYYY-MM-DD HH:MM:SS` form) and `description` (the note text).
  Both are real attributes of the annotation object; the annotation is placed in
  the project from your `MIXPANEL_PROJECT_ID`.

## Step 2: Initialize the directory

Run this once in your new directory:

```sh
terraform init
```

`terraform init` downloads the Mixpanel provider named in your configuration and
sets up a hidden `.terraform` directory to hold it. You run it once per project
(and again whenever you add or upgrade a provider). It makes no changes to
Mixpanel.

## Step 3: Read the plan

```sh
terraform plan
```

This prints the dry run. For the file above, it will look something like this:

```text
Terraform will perform the following actions:

  # mixpanel_annotation.launch will be created
  + resource "mixpanel_annotation" "launch" {
      + annotation_id = (known after apply)
      + date          = "2026-07-08 00:00:00"
      + description   = "Summer launch"
      + id            = (known after apply)
      + project_id    = (known after apply)
      + user          = (known after apply)
      + user_id       = (known after apply)
    }

Plan: 1 to add, 0 to change, 0 to destroy.
```

The symbol at the start of each line is the whole story of a Terraform diff:

- `+` means **create** — this attribute or resource will be added.
- `~` means **change in place** — an existing object will be modified but kept.
- `-` means **destroy** — an object will be deleted.
- `-/+` means **replace** — destroy the old object and create a new one, which
  happens when an attribute cannot be changed without recreating the object.

`(known after apply)` marks values Mixpanel assigns for you (like the
annotation's ID) that Terraform cannot know until the object actually exists. The
summary line — `1 to add, 0 to change, 0 to destroy` — is the count to sanity-check
before you go further.

## Step 4: Apply, then look in the Mixpanel UI

```sh
terraform apply
```

`apply` shows the same plan again and asks you to confirm. Type `yes`. Terraform
makes the API call, and you will see:

```text
mixpanel_annotation.launch: Creating...
mixpanel_annotation.launch: Creation complete after 1s [id=12345]

Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
```

Now open Mixpanel in your browser. The annotation appears on your project's
charts on the date you set — the note you wrote from a text file is live in the
product. That round trip, from `.tf` file to something a teammate can see in the
UI, is the entire point.

## Step 5: Change it and plan again — the loop

The day-to-day rhythm is: **edit the file, plan, apply.** Try it. Change the
`description` in `main.tf`:

```terraform
resource "mixpanel_annotation" "launch" {
  date        = "2026-07-08 00:00:00"
  description = "Summer launch — v2.0"
}
```

Run `terraform plan` again. This time it does not create anything; it recognizes
the annotation already exists (that is *state* doing its job) and shows a
change-in-place:

```text
  # mixpanel_annotation.launch will be updated in-place
  ~ resource "mixpanel_annotation" "launch" {
        id          = 12345
      ~ description = "Summer launch" -> "Summer launch — v2.0"
        # (5 unchanged attributes hidden)
    }

Plan: 0 to add, 1 to change, 0 to destroy.
```

The `~` and the `old -> new` arrow show precisely what will change and nothing
else. Run `terraform apply` and confirm, and the annotation's text updates in
Mixpanel. Edit, plan, apply — that loop is Terraform in daily use.

---

## Step 6: Create something real — a cohort

An annotation is a gentle warm-up. Now create a **cohort** — a saved,
reusable audience — the kind of durable object teams actually want under version
control. Add this block to `main.tf`:

```terraform
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
```

A few things worth understanding here:

- `name` is the cohort's display name. `project_id` is the numeric project it
  belongs to — **replace `1234567` with your own project ID** (the same number
  you put in `MIXPANEL_PROJECT_ID`).
- `groups` describes who is in the cohort. Its shape is free-form JSON, so you
  wrap it in `jsonencode(...)`, a built-in function that turns an HCL object into
  a JSON string for you. This minimal example selects all users. The `event`
  clause inside each group is required — a group without it is rejected by the
  Mixpanel API — so keep that structure even as you change the filters.

Run `terraform plan`. It will show `1 to add` for the cohort (your annotation is
unchanged, so it is not in the plan). Run `terraform apply`, confirm, and the
cohort is created. It appears in Mixpanel's Cohorts list.

> To build richer cohort definitions than the all-users example above, and to
> understand exactly what each field in `groups` means, see the
> [cohort resource reference](../resources/cohort.md).

---

## Step 7: Clean up

Because this was a learning run, tear it all down:

```sh
terraform destroy
```

`terraform destroy` is the inverse of apply: it shows a plan made entirely of
`-` lines — everything Terraform is tracking will be deleted — and, once you
confirm with `yes`, removes both the annotation and the cohort from Mixpanel.
Use it whenever you want to remove everything a configuration manages. (To
delete just one object, remove its block from the file and apply; the plan will
show that single `-`.)

---

## Next steps

You now know the whole loop. Here is where to go from here.

- **Already have objects in the Mixpanel UI?** You do not have to recreate them
  by hand. [Importing existing Mixpanel objects](./import.md) brings a cohort,
  dashboard, or metric you built by clicking under Terraform management, so you
  can manage it as code from then on.
- **Managing more than one project?** [Cross-project portability](./cross-project-portability.md)
  shows how to deploy one configuration across your dev, staging, and production
  projects so they stop drifting apart.
- **Applied something but see nothing in the Mixpanel UI?** This is the single
  most common surprise for newcomers. Objects your service account creates
  (cohorts, dashboards, saved metrics, and more) are **private to that service
  account until they are shared** — so you can apply successfully and still see
  an empty UI, because the objects are visible only to the account that made
  them. [Entity sharing](./sharing.md) explains the `share_with_project`
  attribute that fixes this (it defaults to sharing with the whole project, but
  it is worth knowing the mechanism the moment a created object seems to be
  missing).

For the full catalog of what you can manage — every resource and data source,
grouped by area — see the [provider index](../index.md).

---

## Running this with an AI agent

Everything above is expressed as code and plain shell commands, which is exactly
the shape an AI coding assistant works best in. If you use one, you can drive the
same flow by describing outcomes instead of typing the files yourself. The safety
comes from the plan step: the agent proposes changes, but nothing reaches
Mixpanel until *you* read the plan and confirm the apply.

The steps in this guide map cleanly onto prompts:

- *"Set up a new Terraform project for the Mixpanel provider using my
  `MIXPANEL_*` environment variables, and add an annotation dated today that
  says 'Summer launch'."* — produces Steps 1–3.
- *"Run `terraform plan` and explain what it will do before we apply."* — the
  agent reads the diff back to you in plain language; you decide whether to
  apply.
- *"Add a Power Users cohort that includes all users, then plan it."* — produces
  Step 6, ready for you to review.

Because each change is a `terraform plan` you approve before applying, an agent
can move quickly without moving anything to Mixpanel that you did not see first.
The [analytics-as-code](./analytics-as-code.md) guide covers why this
plan-and-review harness fits agent-proposed change so well.
