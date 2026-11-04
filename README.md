# terraform-provider-nuclock

Manage a [nuclock](https://clock.nuspect.net) organisation as code: its projects,
its people, who sits on which project, and the trackers each project is
connected to. Somebody joining is a block in a pull request; somebody leaving is
that block removed — and nothing is forgotten, because the apply is the only
way it happens.

```hcl
resource "nuclock_user" "ann" {
  email = "ann@example.com"
  roles = ["executor"]
}

resource "nuclock_seat" "ann_on_apollo" {
  project_id = nuclock_project.apollo.id
  user_id    = nuclock_user.ann.id
  position   = "Backend developer"
}
```

## Installing

The provider is published in the
[OpenTofu Registry](https://search.opentofu.org/provider/corneizer/nuclock/latest).
OpenTofu finds it by its short address; Terraform installs from that registry
by hostname:

```hcl
terraform {
  required_providers {
    nuclock = {
      source = "registry.opentofu.org/corneizer/nuclock" # OpenTofu: "corneizer/nuclock"
    }
  }
}
```

## The key

Mint an organisation **service key** in nuclock (Settings → Assistants →
Organisation keys, scope `write`) and tick what it may do. A key holds only the
abilities it was minted with:

| Ability | For |
| --- | --- |
| `projects.write` | `nuclock_project` |
| `people.write` | `nuclock_user` |
| `seats.write` | `nuclock_seat` |
| `integrations.write` | `nuclock_integration` |
| `people.recovery` | the three actions — every one sends the person a letter |

The "for Terraform" preset ticks the first four. A key without the one a
resource needs is refused with `ability_missing`, and the error names the
ability.

```sh
export NUCLOCK_URL=https://api.clock.example.com
export NUCLOCK_API_KEY=nuc_…
```

## What is deliberately not here

* **Rates.** What a seat pays and sells for, and a project's fee and default
  sale price, stay in the tracker. A seat is never sent one and never reads one
  into the state. A project with no default sale price refuses a seat until
  somebody sets one there.
* **Passwords.** Nothing types one and nothing reads one. Creating a person
  mails them a password; `nuclock_reissue_password` mints and mails a new one.
  What a plan can compare is `password_version`, `password_set_at`, the second
  factor's state and `invitation_sent_at`.
* **Clients.** A project's clients are made by the product when they are given
  the project, as an identity provider does it; this provider manages staff.
* **A sign-in link.** No API hands one out, and none should: the invitation
  goes to the person's mailbox (`nuclock_resend_invitation`), not to whoever
  ran the apply.

## Destroy does not delete history

| Resource | Destroy | Declared again |
| --- | --- | --- |
| `nuclock_project` | archived — takes no new work, keeps its hours and invoices | brought back |
| `nuclock_user` | blocked | let back in |
| `nuclock_seat` | closed — the person stops seeing the project, every hour stays | reopened, the same seat |
| `nuclock_integration` | disconnected; imported tasks stay | connected again |

## Secrets

`api_token_wo` and `webhook_secret_wo` are write-only (Terraform 1.11+): sent
when the connection is made, then only when their `_wo_version` changes, and
never stored in the state. The API never hands one back either;
`has_api_token` says whether one is set.

## Finding what nobody manages yet

Every resource has an identity over its natural key — a project's
`key_prefix`, a person's `email`, a seat's `project_id` + `user_id`, a
connection's `project_id` + `kind` + `external_project_id` — so an existing
object is imported rather than twinned:

```hcl
import {
  to       = nuclock_project.apollo
  identity = { key_prefix = "APO" }
}
```

`terraform query` (1.14+) lists projects, people and open seats.

## Requirements

Terraform 1.11+ or OpenTofu 1.11+ for write-only secrets; Terraform 1.14+ for
actions and `terraform query`, which OpenTofu does not have yet. A nuclock new
enough to have seats that close (2026-10-30).

## Developing

```sh
make test     # the resource tests, against an in-memory API
make testacc  # an apply and an empty plan against a real nuclock
make docs     # docs/ from the schema and examples/
```
